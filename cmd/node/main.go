package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "multi_node_platform/api/pb"
)

var (
	nodeID = ""
	token  = ""
	config NodeConfig
)

func negotiateDockerAPI() {
	out, err := exec.Command("docker", "version", "--format", "{{.Server.APIVersion}}").Output()
	if err != nil {
		log.Printf("无法获取 Docker Server API 版本: %v, 跳过协商", err)
		return
	}
	serverAPI := strings.TrimSpace(string(out))
	if serverAPI != "" {
		os.Setenv("DOCKER_API_VERSION", serverAPI)
		log.Printf("Docker API 版本协商: 设置 DOCKER_API_VERSION=%s", serverAPI)
	}
}

func main() {
	config = loadNodeConfig()
	nodeID = config.NodeID

	negotiateDockerAPI()

	conn, err := grpc.Dial(config.ManagerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接 Manager 失败: %v", err)
	}
	defer conn.Close()
	c := pb.NewNodeServiceClient(conn)
	ctx := context.Background()

	token, err = ensureNodeToken(ctx, c, config)
	if err != nil {
		log.Fatalf("节点启动失败: %v", err)
	}
	log.Printf("节点凭证就绪")
	startLogStreamLoop(c)

	// 心跳循环
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		heartbeatRes, err := c.Heartbeat(ctx, &pb.HeartbeatRequest{
			NodeId:    nodeID,
			Token:     token,
			Timestamp: time.Now().Unix(),
		})
		if err != nil {
			log.Printf("心跳失败: %v", err)
			continue
		}
		if !heartbeatRes.Success {
			log.Printf("心跳被拒: %s", heartbeatRes.Message)
			continue
		}
		for _, task := range heartbeatRes.PendingTasks {
			go handleTask(c, task)
		}
	}
}

type ResolvedApp struct {
	Identifier string   `json:"identifier"`
	RunArgs    []string `json:"run_args"`
}

func handleTask(client pb.NodeServiceClient, task *pb.Task) {
	log.Printf("收到任务: %s (Type: %s)", task.TaskId, task.Type)
	ctx := context.Background()

	var resultStatus = "success"
	var errMsg = ""
	var resultJson = "{}"

	switch task.Type {
	case "start_group":
		resultStatus, errMsg = handleStartGroup(task.PayloadJson)
	case "stop_group":
		handleStopGroup(task.PayloadJson)
	case "restart_group":
		resultStatus, errMsg = handleRestartGroup(task.PayloadJson)
	case "replace_proxy":
		resultStatus, errMsg = handleReplaceProxy(task.PayloadJson)
	case "start_browser":
		resultStatus, errMsg, resultJson = handleStartBrowser(task.PayloadJson)
	case "stop_browser":
		handleStopBrowser(task.PayloadJson)
	case "browser_action":
		resultStatus, errMsg, resultJson = handleBrowserAction(task.PayloadJson)
	default:
		resultStatus = "failed"
		errMsg = "未知任务类型: " + task.Type
	}

	client.ReportTaskResult(ctx, &pb.TaskResult{
		TaskId:       task.TaskId,
		Status:       resultStatus,
		ErrorMessage: errMsg,
		ResultJson:   resultJson,
	})
	log.Printf("任务 %s 完成: %s", task.TaskId, resultStatus)
}

// ============================================================
// 隧道启动 — 支持 tun2socks / tun2proxy
// ============================================================

func startTunnel(tunnelType, groupID, proxyURL string) (string, error) {
	tunName := fmt.Sprintf("tunnel-%s", groupID)

	// 先清理可能的残留
	exec.Command("docker", "rm", "-f", tunName).Run()

	var args []string
	switch tunnelType {
	case "tun2proxy":
		args = []string{
			"run", "-d", "--name", tunName, "--restart=always",
			"--cap-add=NET_ADMIN", "--device=/dev/net/tun",
			"ghcr.io/blechschmidt/tun2proxy:latest",
			"--proxy", proxyURL,
		}
	default: // tun2socks
		args = []string{
			"run", "-d", "--name", tunName, "--restart=always",
			"--cap-add=NET_ADMIN",
			"--mount", "type=bind,source=/dev/net/tun,target=/dev/net/tun",
			"-e", fmt.Sprintf("PROXY=%s", proxyURL),
			"xjasonlyu/tun2socks:v2.6.0",
		}
	}

	cmd := exec.Command("docker", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return tunName, fmt.Errorf("启动隧道失败: %v, output: %s", err, string(out))
	}
	return tunName, nil
}

func handleStartGroup(payloadJSON string) (string, string) {
	var payload struct {
		GroupID      string `json:"group_id"`
		TunnelType   string `json:"tunnel_type"`
		ProxyURL     string `json:"proxy_url"`
		ResolvedApps string `json:"resolved_apps"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return "failed", "解析 payload 失败: " + err.Error()
	}

	tunnelType := payload.TunnelType
	if tunnelType == "" {
		tunnelType = "tun2socks"
	}

	tunName, err := startTunnel(tunnelType, payload.GroupID, payload.ProxyURL)
	if err != nil {
		return "failed", err.Error()
	}

	var appList []ResolvedApp
	if payload.ResolvedApps != "" {
		json.Unmarshal([]byte(payload.ResolvedApps), &appList)
	}

	errMsg := ""
	status := "success"
	for _, app := range appList {
		containerName := fmt.Sprintf("app-%s-%s", app.Identifier, payload.GroupID)
		// 清理残留
		exec.Command("docker", "rm", "-f", containerName).Run()

		args := []string{"run", "-d", "--name", containerName, "--restart=always",
			"--network", fmt.Sprintf("container:%s", tunName)}
		args = append(args, app.RunArgs...)

		cmd := exec.Command("docker", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			status = "failed"
			errMsg += fmt.Sprintf("%s 启动失败: %v (%s); ", app.Identifier, err, string(out))
		}
	}
	return status, errMsg
}

func handleStopGroup(payloadJSON string) {
	var payload struct {
		GroupID string `json:"group_id"`
		Apps    string `json:"apps"`
	}
	json.Unmarshal([]byte(payloadJSON), &payload)

	var appList []string
	if payload.Apps != "" && payload.Apps != "[]" {
		json.Unmarshal([]byte(payload.Apps), &appList)
	}

	// 停止所有 app 容器
	for _, appID := range appList {
		exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s-%s", appID, payload.GroupID)).Run()
	}
	// 停止浏览器容器 (名称模式: browser-*)
	out, _ := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=browser-"), "--format", "{{.Names}}").Output()
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name != "" && strings.Contains(name, payload.GroupID) {
			exec.Command("docker", "rm", "-f", name).Run()
		}
	}
	// 停止隧道
	exec.Command("docker", "rm", "-f", fmt.Sprintf("tunnel-%s", payload.GroupID)).Run()
	// 清理旧命名兼容
	exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s", payload.GroupID)).Run()
}

func handleRestartGroup(payloadJSON string) (string, string) {
	var payload struct {
		GroupID string `json:"group_id"`
		Apps    string `json:"apps"`
	}
	json.Unmarshal([]byte(payloadJSON), &payload)

	var appList []string
	if payload.Apps != "" && payload.Apps != "[]" {
		json.Unmarshal([]byte(payload.Apps), &appList)
	}

	errMsg := ""
	status := "success"

	if err := exec.Command("docker", "restart", fmt.Sprintf("tunnel-%s", payload.GroupID)).Run(); err != nil {
		status = "failed"
		errMsg += fmt.Sprintf("隧道重启失败: %v; ", err)
	}

	for _, appID := range appList {
		name := fmt.Sprintf("app-%s-%s", appID, payload.GroupID)
		if err := exec.Command("docker", "restart", name).Run(); err != nil {
			status = "failed"
			errMsg += fmt.Sprintf("%s 重启失败: %v; ", appID, err)
		}
	}
	return status, errMsg
}

func handleReplaceProxy(payloadJSON string) (string, string) {
	var payload struct {
		GroupID      string `json:"group_id"`
		TunnelType   string `json:"tunnel_type"`
		ProxyURL     string `json:"proxy_url"`
		ResolvedApps string `json:"resolved_apps"`
		Apps         string `json:"apps"`
	}
	json.Unmarshal([]byte(payloadJSON), &payload)

	// 停止旧容器
	var rawApps []string
	if payload.Apps != "" && payload.Apps != "[]" {
		json.Unmarshal([]byte(payload.Apps), &rawApps)
	}
	for _, appID := range rawApps {
		exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s-%s", appID, payload.GroupID)).Run()
	}
	exec.Command("docker", "rm", "-f", fmt.Sprintf("tunnel-%s", payload.GroupID)).Run()
	exec.Command("docker", "rm", "-f", fmt.Sprintf("app-%s", payload.GroupID)).Run()

	// 启动新隧道
	tunnelType := payload.TunnelType
	if tunnelType == "" {
		tunnelType = "tun2socks"
	}
	tunName, err := startTunnel(tunnelType, payload.GroupID, payload.ProxyURL)
	if err != nil {
		return "failed", err.Error()
	}

	// 启动 apps
	var appList []ResolvedApp
	if payload.ResolvedApps != "" {
		json.Unmarshal([]byte(payload.ResolvedApps), &appList)
	}

	errMsg := ""
	status := "success"
	for _, app := range appList {
		containerName := fmt.Sprintf("app-%s-%s", app.Identifier, payload.GroupID)
		args := []string{"run", "-d", "--name", containerName, "--restart=always",
			"--network", fmt.Sprintf("container:%s", tunName)}
		args = append(args, app.RunArgs...)
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			status = "failed"
			errMsg += fmt.Sprintf("%s 启动失败: %v (%s); ", app.Identifier, err, string(out))
		}
	}
	return status, errMsg
}

// ============================================================
// 浏览器任务
// ============================================================

type BrowserStartPayload struct {
	BrowserID           string `json:"browser_id"`
	ContainerName       string `json:"container_name"`
	TunnelContainerName string `json:"tunnel_container_name"`
	TunnelType          string `json:"tunnel_type"`
	BrowserImage        string `json:"browser_image"`
	VNCPassword         string `json:"vnc_password"`
	ProxyURL            string `json:"proxy_url"`
}

func startNamedTunnel(tunnelType, tunName, proxyURL string) error {
	exec.Command("docker", "rm", "-f", tunName).Run()

	var args []string
	switch tunnelType {
	case "tun2proxy":
		args = []string{
			"run", "-d", "--name", tunName, "--restart=always",
			"--cap-add=NET_ADMIN", "--device=/dev/net/tun",
			"ghcr.io/blechschmidt/tun2proxy:latest",
			"--proxy", proxyURL,
		}
	default:
		args = []string{
			"run", "-d", "--name", tunName, "--restart=always",
			"--cap-add=NET_ADMIN",
			"--mount", "type=bind,source=/dev/net/tun,target=/dev/net/tun",
			"-e", fmt.Sprintf("PROXY=%s", proxyURL),
			"xjasonlyu/tun2socks:v2.6.0",
		}
	}

	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("启动浏览器专属隧道失败: %v (%s)", err, string(out))
	}
	return nil
}

func buildBrowserRunArgs(payload BrowserStartPayload, tunnelContainerName string) []string {
	args := []string{
		"run", "-d", "--name", payload.ContainerName, "--restart=always",
		"--shm-size=512m",
		"--tmpfs", "/tmp/runtime-kasm:rw,nosuid,nodev,size=64m",
		"-e", fmt.Sprintf("VNC_PW=%s", payload.VNCPassword),
		"-e", "XDG_RUNTIME_DIR=/tmp/runtime-kasm",
		"-p", "0:6901",
		"-p", "0:9222",
	}
	if tunnelContainerName != "" {
		args = append(args, "--network", fmt.Sprintf("container:%s", tunnelContainerName))
	}
	if strings.Contains(payload.BrowserImage, "kasmweb") {
		args = append(args, "-e", "CHROME_ARGS=--remote-debugging-port=9222 --remote-debugging-address=0.0.0.0 --disable-gpu --disable-gpu-compositing --disable-accelerated-2d-canvas --use-gl=swiftshader --disable-dev-shm-usage --no-sandbox")
	}
	args = append(args, payload.BrowserImage)
	return args
}

func handleStartBrowser(payloadJSON string) (string, string, string) {
	var payload BrowserStartPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return "failed", "解析 payload 失败", "{}"
	}

	// 清理残留
	exec.Command("docker", "rm", "-f", payload.ContainerName).Run()
	if payload.TunnelContainerName != "" {
		exec.Command("docker", "rm", "-f", payload.TunnelContainerName).Run()
	}

	tunnelContainerName := ""
	if payload.ProxyURL != "" {
		tunnelType := payload.TunnelType
		if tunnelType == "" {
			tunnelType = "tun2socks"
		}
		tunnelContainerName = payload.TunnelContainerName
		if tunnelContainerName == "" {
			tunnelContainerName = fmt.Sprintf("browser-tunnel-%s", payload.BrowserID[:8])
		}
		if err := startNamedTunnel(tunnelType, tunnelContainerName, payload.ProxyURL); err != nil {
			return "failed", err.Error(), "{}"
		}
	}

	args := buildBrowserRunArgs(payload, tunnelContainerName)

	cmd := exec.Command("docker", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if tunnelContainerName != "" {
			exec.Command("docker", "rm", "-f", tunnelContainerName).Run()
		}
		return "failed", fmt.Sprintf("浏览器启动失败: %v (%s)", err, string(out)), "{}"
	}

	// 获取实际映射端口
	vncPort := getHostPort(payload.ContainerName, "6901")
	cdpPort := getHostPort(payload.ContainerName, "9222")

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"vnc_port": vncPort,
		"cdp_port": cdpPort,
	})
	return "success", "", string(resultJSON)
}

func handleStopBrowser(payloadJSON string) {
	var payload struct {
		ContainerName       string `json:"container_name"`
		TunnelContainerName string `json:"tunnel_container_name"`
	}
	json.Unmarshal([]byte(payloadJSON), &payload)
	exec.Command("docker", "rm", "-f", payload.ContainerName).Run()
	if payload.TunnelContainerName != "" {
		exec.Command("docker", "rm", "-f", payload.TunnelContainerName).Run()
	}
}

func handleBrowserAction(payloadJSON string) (string, string, string) {
	var payload struct {
		ContainerName string `json:"container_name"`
		Action        string `json:"action"`
		Params        string `json:"params"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return "failed", "解析失败", "{}"
	}

	var params map[string]string
	json.Unmarshal([]byte(payload.Params), &params)

	switch payload.Action {
	case "screenshot":
		// 通过 docker exec 使用 scrot 或 xdotool 截图
		out, err := exec.Command("docker", "exec", payload.ContainerName,
			"bash", "-c", "DISPLAY=:1 import -window root /tmp/screenshot.png && base64 /tmp/screenshot.png").CombinedOutput()
		if err != nil {
			return "failed", fmt.Sprintf("截图失败: %v", err), "{}"
		}
		result, _ := json.Marshal(map[string]string{"screenshot_base64": string(out)})
		return "success", "", string(result)

	case "navigate":
		url := params["url"]
		if url == "" {
			return "failed", "缺少 url 参数", "{}"
		}
		// 通过 xdotool 打开 URL
		exec.Command("docker", "exec", payload.ContainerName,
			"bash", "-c", fmt.Sprintf("DISPLAY=:1 chromium --no-sandbox '%s' &", url)).Run()
		return "success", "", "{}"

	case "exec_js":
		script := params["script"]
		if script == "" {
			return "failed", "缺少 script 参数", "{}"
		}
		// 预留：通过 CDP 执行 JS
		return "success", "exec_js 暂未实现 CDP 集成", "{}"

	default:
		return "failed", "未知 action: " + payload.Action, "{}"
	}
}

// getHostPort 获取容器的宿主机映射端口
func getHostPort(containerName, containerPort string) int {
	out, err := exec.Command("docker", "port", containerName, containerPort).Output()
	if err != nil {
		return 0
	}
	// 输出格式: 0.0.0.0:32768 或 :::32768
	parts := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(parts) >= 2 {
		port := 0
		fmt.Sscanf(parts[len(parts)-1], "%d", &port)
		return port
	}
	return 0
}
