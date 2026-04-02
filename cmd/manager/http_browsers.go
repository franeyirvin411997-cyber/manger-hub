package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"multi_node_platform/pkg/models"
)

func getBrowsers(c *gin.Context) {
	nodeID := c.Query("node_id")
	accountID := c.Query("account_id")
	proxyID := c.Query("proxy_resource_id")

	q := models.DB.Model(&models.BrowserInstance{})
	if nodeID != "" {
		q = q.Where("node_id = ?", nodeID)
	}
	if accountID != "" {
		q = q.Where("account_id = ?", accountID)
	}
	if proxyID != "" {
		q = q.Where("proxy_resource_id = ?", proxyID)
	}

	var browsers []models.BrowserInstance
	q.Find(&browsers)

	// 联查代理信息
	type BrowserView struct {
		models.BrowserInstance
		ProxyHost     string `json:"proxy_host"`
		ProxyPort     int    `json:"proxy_port"`
		ProxyProtocol string `json:"proxy_protocol"`
	}
	var result []BrowserView
	for _, b := range browsers {
		bv := BrowserView{BrowserInstance: b}
		if b.ProxyResourceID != "" {
			var proxy models.ProxyResource
			if models.DB.Where("id = ?", b.ProxyResourceID).First(&proxy).Error == nil {
				bv.ProxyHost = proxy.Host
				bv.ProxyPort = proxy.Port
				bv.ProxyProtocol = proxy.Protocol
			}
		}
		result = append(result, bv)
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func createBrowser(c *gin.Context) {
	var req struct {
		DisplayName string `json:"display_name"`
		NodeID      string `json:"node_id" binding:"required"`
		ProxyID     string `json:"proxy_id"`   // 可选：直接指定代理
		AccountID   string `json:"account_id"` // 可选绑定
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证节点存在
	var node models.Node
	if err := models.DB.Where("id = ?", req.NodeID).First(&node).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "节点不存在"})
		return
	}

	// 构建代理 URL（如果指定了代理）
	proxyURL := ""
	proxyResourceID := ""
	if req.ProxyID != "" {
		var proxy models.ProxyResource
		if err := models.DB.Where("id = ?", req.ProxyID).First(&proxy).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "代理不存在"})
			return
		}
		proxyURL = buildProxyURL(proxy)
		proxyResourceID = proxy.ID
	}

	browserImage := models.GetConfig("browser_image")
	if browserImage == "" {
		browserImage = "kasmweb/chromium:1.16.1"
	}
	vncPassword := models.GetConfig("browser_vnc_password")
	if vncPassword == "" {
		vncPassword = "changeme"
	}

	browserID := uuid.New().String()
	tunnelType := ""
	tunnelContainerName := ""
	if proxyURL != "" {
		tunnelType = models.GetConfig("default_tunnel_type")
		if tunnelType == "" {
			tunnelType = "tun2socks"
		}
		tunnelContainerName = fmt.Sprintf("browser-tunnel-%s", browserID[:8])
	}
	browser := models.BrowserInstance{
		ID:                  browserID,
		DisplayName:         req.DisplayName,
		NodeID:              req.NodeID,
		ProxyResourceID:     proxyResourceID,
		AccountID:           req.AccountID,
		ContainerName:       fmt.Sprintf("browser-%s", browserID[:8]),
		TunnelContainerName: tunnelContainerName,
		TunnelType:          tunnelType,
		BrowserImage:        browserImage,
		VNCPassword:         vncPassword,
		Status:              "pending",
	}

	// 创建部署任务
	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "create_browser",
		TargetID: browserID,
		Status:   "pending",
		Operator: "admin",
	}

	payloadMap := map[string]string{
		"browser_id":            browserID,
		"container_name":        browser.ContainerName,
		"tunnel_container_name": browser.TunnelContainerName,
		"tunnel_type":           browser.TunnelType,
		"browser_image":         browserImage,
		"vnc_password":          vncPassword,
		"proxy_url":             proxyURL,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	task := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      req.NodeID,
		Type:        "start_browser",
		Payload:     string(payloadBytes),
		Status:      "pending",
		ScopeKey:    fmt.Sprintf("browser:%s:start", browserID),
		Timeout:     300,
	}

	models.DB.Create(&browser)
	models.DB.Create(&op)
	models.DB.Create(&task)

	c.JSON(http.StatusOK, gin.H{"message": "浏览器创建指令已下发", "data": map[string]string{"id": browserID}})
}

func stopBrowser(c *gin.Context) {
	id := c.Param("id")
	var browser models.BrowserInstance
	if err := models.DB.Where("id = ?", id).First(&browser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "浏览器不存在"})
		return
	}

	op := models.Operation{
		ID: uuid.New().String(), Type: "stop_browser", TargetID: id, Status: "pending", Operator: "admin",
	}
	payloadMap := map[string]string{
		"browser_id":            id,
		"container_name":        browser.ContainerName,
		"tunnel_container_name": browser.TunnelContainerName,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: browser.NodeID,
		Type: "stop_browser", Payload: string(payloadBytes), Status: "pending",
		ScopeKey: fmt.Sprintf("browser:%s:stop", id), Timeout: 120,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)
	models.DB.Model(&browser).Update("status", "stopped")

	c.JSON(http.StatusOK, gin.H{"message": "浏览器停止指令已下发"})
}

func deleteBrowser(c *gin.Context) {
	id := c.Param("id")
	var browser models.BrowserInstance
	if err := models.DB.Where("id = ?", id).First(&browser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "浏览器不存在"})
		return
	}

	// 先下发停止任务
	op := models.Operation{
		ID: uuid.New().String(), Type: "delete_browser", TargetID: id, Status: "pending", Operator: "admin",
	}
	payloadMap := map[string]string{
		"browser_id":            id,
		"container_name":        browser.ContainerName,
		"tunnel_container_name": browser.TunnelContainerName,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: browser.NodeID,
		Type: "stop_browser", Payload: string(payloadBytes), Status: "pending", Timeout: 120,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)
	models.DB.Where("id = ?", id).Delete(&models.BrowserInstance{})

	c.JSON(http.StatusOK, gin.H{"message": "浏览器已删除"})
}

func bindBrowserAccount(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		AccountID string `json:"account_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var browser models.BrowserInstance
	if err := models.DB.Where("id = ?", id).First(&browser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "浏览器不存在"})
		return
	}

	models.DB.Model(&browser).Update("account_id", req.AccountID)
	c.JSON(http.StatusOK, gin.H{"message": "浏览器已绑定账号"})
}

// proxyBrowserVNC 代理浏览器 VNC 连接（WebSocket 和 HTTP）
func proxyBrowserVNC(c *gin.Context) {
	id := c.Param("id")
	var browser models.BrowserInstance
	if err := models.DB.Where("id = ?", id).First(&browser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "浏览器不存在"})
		return
	}
	if browser.VNCPort == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "浏览器 VNC 端口尚未就绪"})
		return
	}

	var node models.Node
	if err := models.DB.Where("id = ?", browser.NodeID).First(&node).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "节点信息不可用"})
		return
	}

	// 返回 VNC 连接信息让前端直连（或通过 nginx 代理）
	targetURL := getVNCProxyURL(node.IP, browser.VNCPort)
	c.JSON(http.StatusOK, gin.H{
		"vnc_url":      targetURL,
		"vnc_password": browser.VNCPassword,
		"node_ip":      node.IP,
		"vnc_port":     browser.VNCPort,
	})
}

// browserExecAction 在浏览器容器中执行自动化操作
func browserExecAction(c *gin.Context) {
	id := c.Param("id")
	var browser models.BrowserInstance
	if err := models.DB.Where("id = ?", id).First(&browser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "浏览器不存在"})
		return
	}

	var req struct {
		Action string `json:"action" binding:"required"` // navigate, screenshot, click, type, exec_js
		Params string `json:"params"`                    // JSON 参数
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	op := models.Operation{
		ID: uuid.New().String(), Type: "browser_action", TargetID: id, Status: "pending", Operator: "admin",
	}
	payloadMap := map[string]string{
		"browser_id":     id,
		"container_name": browser.ContainerName,
		"action":         req.Action,
		"params":         req.Params,
		"cdp_port":       fmt.Sprintf("%d", browser.CDPPort),
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: browser.NodeID,
		Type: "browser_action", Payload: string(payloadBytes), Status: "pending", Timeout: 60,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)

	c.JSON(http.StatusOK, gin.H{"message": "浏览器操作指令已下发", "task_id": task.ID})
}

// getBrowserScreenshot 获取浏览器截图（从 Node 的 CDP 端口获取）
func getBrowserScreenshot(c *gin.Context) {
	id := c.Param("id")
	var browser models.BrowserInstance
	if err := models.DB.Where("id = ?", id).First(&browser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "浏览器不存在"})
		return
	}

	var node models.Node
	if err := models.DB.Where("id = ?", browser.NodeID).First(&node).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "节点信息不可用"})
		return
	}

	// 通过 CDP 获取截图（node 上的 CDP 端口）
	if browser.CDPPort == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "CDP 端口未就绪"})
		return
	}

	cdpURL := fmt.Sprintf("http://%s:%d/screenshot", node.IP, browser.CDPPort)
	resp, err := http.Get(cdpURL)
	if err != nil {
		log.Printf("获取截图失败: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "无法连接浏览器"})
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", "image/png")
	io.Copy(c.Writer, resp.Body)
}

// getVNCProxyURL 生成安全的 VNC 代理 URL
func getVNCProxyURL(nodeIP string, vncPort int) string {
	basePath := fmt.Sprintf("/vnc/%s/%d/", url.PathEscape(nodeIP), vncPort)
	query := url.Values{}
	query.Set("path", fmt.Sprintf("vnc/%s/%d/websockify", nodeIP, vncPort))
	return basePath + "?" + query.Encode()
}
