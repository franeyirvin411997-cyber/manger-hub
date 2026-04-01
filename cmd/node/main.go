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

	if task.Type == "start_group" {
		var payload map[string]string
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload["group_id"]
			proxyURL := payload["proxy_url"] // 例如: socks5://ip:port

			// 1. 启动 Gost 容器作为本地隧道
			gostContainerName := fmt.Sprintf("tunnel-%s", groupID)
			gostCmd := exec.Command("docker", "run", "-d", "--name", gostContainerName, "ginuerzh/gost", "-L", ":1080", "-F", proxyURL)
			if err := gostCmd.Run(); err != nil {
				resultStatus = "failed"
				errMsg = fmt.Sprintf("Failed to start gost tunnel: %v", err)
			} else {
				// 2. 启动 App 容器，共享 Gost 容器的网络命名空间，并注入代理环境变量
				appContainerName := fmt.Sprintf("app-%s", groupID)
				appCmd := exec.Command("docker", "run", "-d", "--name", appContainerName,
					"--network", fmt.Sprintf("container:%s", gostContainerName),
					"-e", "http_proxy=http://127.0.0.1:1080",
					"-e", "https_proxy=http://127.0.0.1:1080",
					"-e", "all_proxy=socks5://127.0.0.1:1080",
					"alpine", "sleep", "3600")
				if err := appCmd.Run(); err != nil {
					resultStatus = "failed"
					errMsg = fmt.Sprintf("Failed to start app container: %v", err)
				}
			}
		} else {
			resultStatus = "failed"
			errMsg = "Invalid payload format"
		}
	} else if task.Type == "stop_group" {
		var payload map[string]string
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload["group_id"]

			// 停止并删除容器
			appContainerName := fmt.Sprintf("app-%s", groupID)
			gostContainerName := fmt.Sprintf("tunnel-%s", groupID)

			exec.Command("docker", "rm", "-f", appContainerName).Run()
			exec.Command("docker", "rm", "-f", gostContainerName).Run()
		}
	} else if task.Type == "replace_proxy" {
		var payload map[string]string
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload["group_id"]
			proxyURL := payload["proxy_url"]

			gostContainerName := fmt.Sprintf("tunnel-%s", groupID)
			appContainerName := fmt.Sprintf("app-%s", groupID)

			// 对于隧道容器，最简单的替换方式是删掉重建，App容器也需要跟着重启以重连网络
			exec.Command("docker", "rm", "-f", appContainerName).Run()
			exec.Command("docker", "rm", "-f", gostContainerName).Run()

			gostCmd := exec.Command("docker", "run", "-d", "--name", gostContainerName, "ginuerzh/gost", "-L", ":1080", "-F", proxyURL)
			if err := gostCmd.Run(); err != nil {
				resultStatus = "failed"
				errMsg = fmt.Sprintf("Failed to restart gost tunnel: %v", err)
			} else {
				appCmd := exec.Command("docker", "run", "-d", "--name", appContainerName,
					"--network", fmt.Sprintf("container:%s", gostContainerName),
					"-e", "http_proxy=http://127.0.0.1:1080",
					"-e", "https_proxy=http://127.0.0.1:1080",
					"-e", "all_proxy=socks5://127.0.0.1:1080",
					"alpine", "sleep", "3600")
				if err := appCmd.Run(); err != nil {
					resultStatus = "failed"
					errMsg = fmt.Sprintf("Failed to restart app container: %v", err)
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
