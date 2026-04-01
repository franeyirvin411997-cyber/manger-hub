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
		var payload struct {
			GroupID  string `json:"group_id"`
			ProxyURL string `json:"proxy_url"`
			Apps     string `json:"apps"` // JSON array of app identifiers
			AppConfigs string `json:"app_configs"` // JSON map of configs
		}
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload.GroupID
			proxyURL := payload.ProxyURL

			// 1. 启动 Gost 容器作为本地隧道
			gostContainerName := fmt.Sprintf("tunnel-%s", groupID)
			gostCmd := exec.Command("docker", "run", "-d", "--name", gostContainerName, "ginuerzh/gost", "-L", ":1080", "-F", proxyURL)
			if err := gostCmd.Run(); err != nil {
				resultStatus = "failed"
				errMsg = fmt.Sprintf("Failed to start gost tunnel: %v", err)
			} else {
				// 解析应用清单
				var appList []string
				if payload.Apps != "" && payload.Apps != "[]" {
					json.Unmarshal([]byte(payload.Apps), &appList)
				} else {
					// 默认后备用于旧数据兼容
					appList = []string{"alpine"}
				}

				var appConfigs map[string]string
				if payload.AppConfigs != "" {
					json.Unmarshal([]byte(payload.AppConfigs), &appConfigs)
				} else {
					appConfigs = make(map[string]string)
				}

				// 2. 为每个选中的 App 启动一个容器，共享 Gost 容器的网络
				for _, appIdentifier := range appList {
					appContainerName := fmt.Sprintf("app-%s-%s", appIdentifier, groupID)

					var appCmd *exec.Cmd
					baseArgs := []string{
						"run", "-d", "--name", appContainerName,
						"--network", fmt.Sprintf("container:%s", gostContainerName),
						"-e", "http_proxy=http://127.0.0.1:1080",
						"-e", "https_proxy=http://127.0.0.1:1080",
						"-e", "all_proxy=socks5://127.0.0.1:1080",
					}

					// 根据应用类型适配启动参数 (可根据实际 Docker 镜像参数调整)
					switch appIdentifier {
					case "traffmonetizer":
						token := appConfigs["traffmonetizer_token"]
						baseArgs = append(baseArgs, "traffmonetizer/cli_v2:latest", "start", "accept", "--token", token)
					case "repocket":
						email := appConfigs["repocket_email"]
						apiKey := appConfigs["repocket_api_key"]
						baseArgs = append(baseArgs, "-e", fmt.Sprintf("RP_EMAIL=%s", email), "-e", fmt.Sprintf("RP_API_KEY=%s", apiKey), "repocket/repocket:latest")
					case "honeygain":
						email := appConfigs["honeygain_email"]
						password := appConfigs["honeygain_password"]
						baseArgs = append(baseArgs, "honeygain/honeygain:latest", "-tou-accept", "-email", email, "-pass", password, "-device", groupID)
					case "packetstream":
						cid := appConfigs["packetstream_cid"]
						baseArgs = append(baseArgs, "-e", fmt.Sprintf("CID=%s", cid), "packetstream/psclient:latest")
					default: // 默认 alpine (测试用)
						baseArgs = append(baseArgs, "alpine", "sleep", "3600")
					}

					appCmd = exec.Command("docker", baseArgs...)
					if err := appCmd.Run(); err != nil {
						resultStatus = "failed"
						errMsg += fmt.Sprintf("Failed to start %s container: %v; ", appIdentifier, err)
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

			gostContainerName := fmt.Sprintf("tunnel-%s", groupID)
			exec.Command("docker", "rm", "-f", gostContainerName).Run()

			// 清理遗留容器（兼容以前单一app命名的容器）
			exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s", groupID)).Run()
		}
	} else if task.Type == "replace_proxy" {
		var payload struct {
			GroupID    string `json:"group_id"`
			ProxyURL   string `json:"proxy_url"`
			Apps       string `json:"apps"`
			AppConfigs string `json:"app_configs"`
		}
		if err := json.Unmarshal([]byte(task.PayloadJson), &payload); err == nil {
			groupID := payload.GroupID
			proxyURL := payload.ProxyURL

			// 解析应用清单
			var appList []string
			if payload.Apps != "" && payload.Apps != "[]" {
				json.Unmarshal([]byte(payload.Apps), &appList)
			} else {
				appList = []string{"alpine"}
			}

			var appConfigs map[string]string
			if payload.AppConfigs != "" {
				json.Unmarshal([]byte(payload.AppConfigs), &appConfigs)
			} else {
				appConfigs = make(map[string]string)
			}

			// 停止旧容器
			gostContainerName := fmt.Sprintf("tunnel-%s", groupID)
			exec.Command("docker", "rm", "-f", gostContainerName).Run()
			exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s", groupID)).Run() // 遗留清理
			for _, appIdentifier := range appList {
				appContainerName := fmt.Sprintf("app-%s-%s", appIdentifier, groupID)
				exec.Command("docker", "rm", "-f", appContainerName).Run()
			}

			gostCmd := exec.Command("docker", "run", "-d", "--name", gostContainerName, "ginuerzh/gost", "-L", ":1080", "-F", proxyURL)
			if err := gostCmd.Run(); err != nil {
				resultStatus = "failed"
				errMsg = fmt.Sprintf("Failed to restart gost tunnel: %v", err)
			} else {
				// 重启选中的应用
				for _, appIdentifier := range appList {
					appContainerName := fmt.Sprintf("app-%s-%s", appIdentifier, groupID)
					var appCmd *exec.Cmd
					baseArgs := []string{
						"run", "-d", "--name", appContainerName,
						"--network", fmt.Sprintf("container:%s", gostContainerName),
						"-e", "http_proxy=http://127.0.0.1:1080",
						"-e", "https_proxy=http://127.0.0.1:1080",
						"-e", "all_proxy=socks5://127.0.0.1:1080",
					}

					switch appIdentifier {
					case "traffmonetizer":
						token := appConfigs["traffmonetizer_token"]
						baseArgs = append(baseArgs, "traffmonetizer/cli_v2:latest", "start", "accept", "--token", token)
					case "repocket":
						email := appConfigs["repocket_email"]
						apiKey := appConfigs["repocket_api_key"]
						baseArgs = append(baseArgs, "-e", fmt.Sprintf("RP_EMAIL=%s", email), "-e", fmt.Sprintf("RP_API_KEY=%s", apiKey), "repocket/repocket:latest")
					case "honeygain":
						email := appConfigs["honeygain_email"]
						password := appConfigs["honeygain_password"]
						baseArgs = append(baseArgs, "honeygain/honeygain:latest", "-tou-accept", "-email", email, "-pass", password, "-device", groupID)
					case "packetstream":
						cid := appConfigs["packetstream_cid"]
						baseArgs = append(baseArgs, "-e", fmt.Sprintf("CID=%s", cid), "packetstream/psclient:latest")
					default:
						baseArgs = append(baseArgs, "alpine", "sleep", "3600")
					}

					appCmd = exec.Command("docker", baseArgs...)
					if err := appCmd.Run(); err != nil {
						resultStatus = "failed"
						errMsg += fmt.Sprintf("Failed to restart %s container: %v; ", appIdentifier, err)
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
