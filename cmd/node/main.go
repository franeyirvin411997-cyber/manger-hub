package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "multi_node_platform/api/pb"
)

var (
	nodeID    = "node-" + os.Getenv("HOSTNAME")
	token     = ""
	managerIP = os.Getenv("MANAGER_IP")
)

func main() {
	if managerIP == "" {
		managerIP = "localhost"
	}

	conn, err := grpc.Dial(managerIP+":50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewNodeServiceClient(conn)

	ctx := context.Background()

	// 1. 节点注册
	registerRes, err := c.Register(ctx, &pb.RegisterRequest{
		NodeId:       nodeID,
		Hostname:     os.Getenv("HOSTNAME"),
		Version:      "v1.0.0",
		Capabilities: []string{"docker", "gost"},
	})
	if err != nil {
		log.Fatalf("could not register: %v", err)
	}
	if !registerRes.Success {
		log.Fatalf("Registration failed: %s", registerRes.Message)
	}
	token = registerRes.Token
	log.Printf("Successfully registered with token: %s", token)

	// 2. 心跳循环与任务处理
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		heartbeatRes, err := c.Heartbeat(ctx, &pb.HeartbeatRequest{
			NodeId:    nodeID,
			Token:     token,
			Timestamp: time.Now().Unix(),
		})
		if err != nil {
			log.Printf("Heartbeat failed: %v", err)
			continue
		}

		if len(heartbeatRes.PendingTasks) > 0 {
			for _, task := range heartbeatRes.PendingTasks {
				go handleTask(c, task)
			}
		}
	}
}

func handleTask(client pb.NodeServiceClient, task *pb.Task) {
	log.Printf("Received task: %s (Type: %s)", task.TaskId, task.Type)
	ctx := context.Background()

	var resultStatus = "success"
	var errMsg = ""
	var resultJson = "{}"

	// ResolvedApp 代表从 Manager 下发的预解析应用参数
	type ResolvedApp struct {
		Identifier string   `json:"identifier"`
		RunArgs    []string `json:"run_args"`
	}

	if task.Type == "start_group" {
		var payload struct {
			GroupID      string `json:"group_id"`
			ProxyURL     string `json:"proxy_url"`
			ResolvedApps string `json:"resolved_apps"` // JSON array of ResolvedApp
		}
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload.GroupID
			proxyURL := payload.ProxyURL

			// 1. 启动 Tun2Socks 容器作为透明本地网关隧道 (替代代理软件的 HTTP_PROXY 局限)
			tunContainerName := fmt.Sprintf("tunnel-%s", groupID)
			tunCmd := exec.Command("docker", "run", "-d", "--name", tunContainerName,
				"--restart=always",
				"--mount", "type=bind,source=/dev/net/tun,target=/dev/net/tun",
				"--cap-add=NET_ADMIN",
				"-e", fmt.Sprintf("PROXY=%s", proxyURL),
				"xjasonlyu/tun2socks:v2.6.0")

			if err := tunCmd.Run(); err != nil {
				resultStatus = "failed"
				errMsg = fmt.Sprintf("Failed to start tun2socks tunnel: %v", err)
			} else {
				// 解析已经预处理好的应用清单
				var appList []ResolvedApp
				if payload.ResolvedApps != "" {
					json.Unmarshal([]byte(payload.ResolvedApps), &appList)
				}

				// 2. 为每个选中的 App 启动一个容器，完全与业务逻辑解耦
				for _, appInfo := range appList {
					appContainerName := fmt.Sprintf("app-%s-%s", appInfo.Identifier, groupID)

					baseArgs := []string{
						"run", "-d", "--name", appContainerName,
						"--restart=always",
						"--network", fmt.Sprintf("container:%s", tunContainerName),
					}

					baseArgs = append(baseArgs, appInfo.RunArgs...)

					appCmd := exec.Command("docker", baseArgs...)
					if err := appCmd.Run(); err != nil {
						resultStatus = "failed"
						errMsg += fmt.Sprintf("Failed to start %s container: %v; ", appInfo.Identifier, err)
					}
				}
			}
		} else {
			resultStatus = "failed"
			errMsg = "Invalid payload format"
		}
	} else if task.Type == "stop_group" {
		var payload struct {
			GroupID string `json:"group_id"`
			Apps    string `json:"apps"`
		}
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload.GroupID

			// 找出可能启动过的应用容器名字并删除
			var appList []string
			if payload.Apps != "" && payload.Apps != "[]" {
				json.Unmarshal([]byte(payload.Apps), &appList)
			} else {
				appList = []string{"alpine"}
			}

			for _, appIdentifier := range appList {
				appContainerName := fmt.Sprintf("app-%s-%s", appIdentifier, groupID)
				exec.Command("docker", "rm", "-f", appContainerName).Run()
			}

			tunContainerName := fmt.Sprintf("tunnel-%s", groupID)
			exec.Command("docker", "rm", "-f", tunContainerName).Run()

			// 清理遗留容器（兼容以前单一app命名的容器）
			exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s", groupID)).Run()
		}
	} else if task.Type == "replace_proxy" {
		var payload struct {
			GroupID      string `json:"group_id"`
			ProxyURL     string `json:"proxy_url"`
			ResolvedApps string `json:"resolved_apps"` // JSON array of ResolvedApp
			Apps         string `json:"apps"`
		}
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload.GroupID
			proxyURL := payload.ProxyURL

			// 解析已经预处理好的应用清单
			var appList []ResolvedApp
			if payload.ResolvedApps != "" {
				json.Unmarshal([]byte(payload.ResolvedApps), &appList)
			}

			// 解析原始用于清理遗留
			var rawAppList []string
			if payload.Apps != "" && payload.Apps != "[]" {
				json.Unmarshal([]byte(payload.Apps), &rawAppList)
			} else {
				rawAppList = []string{"alpine"}
			}

			// 停止旧容器
			tunContainerName := fmt.Sprintf("tunnel-%s", groupID)
			exec.Command("docker", "rm", "-f", tunContainerName).Run()
			exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s", groupID)).Run() // 遗留清理
			for _, appIdentifier := range rawAppList {
				appContainerName := fmt.Sprintf("app-%s-%s", appIdentifier, groupID)
				exec.Command("docker", "rm", "-f", appContainerName).Run()
			}

			tunCmd := exec.Command("docker", "run", "-d", "--name", tunContainerName,
				"--restart=always",
				"--mount", "type=bind,source=/dev/net/tun,target=/dev/net/tun",
				"--cap-add=NET_ADMIN",
				"-e", fmt.Sprintf("PROXY=%s", proxyURL),
				"xjasonlyu/tun2socks:v2.6.0")

			if err := tunCmd.Run(); err != nil {
				resultStatus = "failed"
				errMsg = fmt.Sprintf("Failed to restart tun2socks tunnel: %v", err)
			} else {
				// 重启选中的应用
				for _, appInfo := range appList {
					appContainerName := fmt.Sprintf("app-%s-%s", appInfo.Identifier, groupID)

					baseArgs := []string{
						"run", "-d", "--name", appContainerName,
						"--restart=always",
						"--network", fmt.Sprintf("container:%s", tunContainerName),
					}

					baseArgs = append(baseArgs, appInfo.RunArgs...)

					appCmd := exec.Command("docker", baseArgs...)
					if err := appCmd.Run(); err != nil {
						resultStatus = "failed"
						errMsg += fmt.Sprintf("Failed to restart %s container: %v; ", appInfo.Identifier, err)
					}
				}
			}
		} else {
			resultStatus = "failed"
			errMsg = "Invalid payload format"
		}
	} else {
		resultStatus = "failed"
		errMsg = "Unknown task type"
	}

	// 上报结果
	client.ReportTaskResult(ctx, &pb.TaskResult{
		TaskId:       task.TaskId,
		Status:       resultStatus,
		ErrorMessage: errMsg,
		ResultJson:   resultJson,
	})
	log.Printf("Task %s completed with status: %s", task.TaskId, resultStatus)
}
