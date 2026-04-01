package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"multi_node_platform/pkg/models"
)

// SetupGinRouter 设置基于 Gin 的 REST API 路由
func SetupGinRouter() *gin.Engine {
	r := gin.Default()

	r.Use(CorsMiddleware())

	adminUser := os.Getenv("ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "admin"
	}

	// 统一的 Admin Basic Auth
	adminGroup := r.Group("/api/v1", gin.BasicAuth(gin.Accounts{
		adminUser: adminPassword, // 从环境变量配置，默认为 admin:admin
	}))

	// 用于前端测试登录凭证是否正确
	adminGroup.GET("/auth", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "authenticated"})
	})

	{
		// 节点管理
		adminGroup.GET("/nodes", getNodes)

		// 代理池管理
		adminGroup.GET("/proxies", getProxies)
		adminGroup.POST("/proxies", addProxy)
		adminGroup.POST("/proxies/csv", uploadProxiesCSV)

		// 代理组管理
		adminGroup.GET("/groups", getGroups)
		adminGroup.POST("/groups", createGroup)
		adminGroup.POST("/groups/:id/replace_proxy", replaceProxy)
		adminGroup.POST("/groups/:id/migrate_node", migrateNode)

		// 操作审计
		adminGroup.GET("/operations", getOperations)
		adminGroup.GET("/tasks", getTasks)

		// 简单的系统信息
		adminGroup.GET("/info", getSystemInfo)

		// 规则管理
		adminGroup.GET("/rules", getRules)
		adminGroup.POST("/rules", addRule)
		adminGroup.DELETE("/rules/:id", deleteRule)
	}

	// 提供节点执行端二进制下载
	// 在 Docker 容器中它被放到了 /usr/local/bin/node
	r.StaticFile("/download/node", "/usr/local/bin/node")

	return r
}

func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func getNodes(c *gin.Context) {
	var nodes []models.Node
	models.DB.Find(&nodes)
	c.JSON(http.StatusOK, gin.H{"data": nodes})
}

func getProxies(c *gin.Context) {
	var proxies []models.ProxyResource
	models.DB.Find(&proxies)
	c.JSON(http.StatusOK, gin.H{"data": proxies})
}

func addProxy(c *gin.Context) {
	var req struct {
		Host     string `json:"host" binding:"required"`
		Port     int    `json:"port" binding:"required"`
		Username string `json:"username"`
		Password string `json:"password"`
		Protocol string `json:"protocol"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	proxy := models.ProxyResource{
		ID:       uuid.New().String(),
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		Protocol: req.Protocol,
		PoolType: "observer",
		Status:   "unknown",
	}

	models.DB.Create(&proxy)
	c.JSON(http.StatusOK, gin.H{"message": "Proxy added", "data": proxy})
}

func uploadProxiesCSV(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "获取文件失败"})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开文件"})
		return
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析 CSV 失败"})
		return
	}

	count := 0
	for i, record := range records {
		if i == 0 {
			continue // 假设第一行是表头
		}
		// CSV 格式约定: Protocol,Host,Port,Username,Password
		if len(record) < 3 {
			continue
		}

		port, _ := strconv.Atoi(record[2])
		proxy := models.ProxyResource{
			ID:       uuid.New().String(),
			Protocol: record[0],
			Host:     record[1],
			Port:     port,
			Username: "",
			Password: "",
			PoolType: "observer",
			Status:   "unknown",
		}
		if len(record) >= 5 {
			proxy.Username = record[3]
			proxy.Password = record[4]
		}
		models.DB.Create(&proxy)
		count++
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("成功导入 %d 条代理", count)})
}

func getGroups(c *gin.Context) {
	var specs []models.GroupSpec
	models.DB.Find(&specs)
	c.JSON(http.StatusOK, gin.H{"data": specs})
}

func createGroup(c *gin.Context) {
	var req struct {
		NodeID     string `json:"node_id" binding:"required"`
		TunnelType string `json:"tunnel_type"`
		Apps       string `json:"apps"` // JSON
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	groupID := uuid.New().String()

	// 查找一个未使用的在线代理 (必须不为 in_use)
	var proxy models.ProxyResource
	if err := models.DB.Where("status = ? AND status != ?", "online", "in_use").First(&proxy).Error; err != nil {
		// 回退查找任何状态未知或离线但未被占用的代理
		if err := models.DB.Where("status != ?", "in_use").First(&proxy).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No available proxies"})
			return
		}
	}

	// 标记为正在使用 (简化实现)
	proxy.Status = "in_use"
	models.DB.Save(&proxy)

	// 构建代理 URL (例如 socks5://user:pass@host:port)
	auth := ""
	if proxy.Username != "" && proxy.Password != "" {
		auth = fmt.Sprintf("%s:%s@", proxy.Username, proxy.Password)
	}
	proxyURL := fmt.Sprintf("%s://%s%s:%d", proxy.Protocol, auth, proxy.Host, proxy.Port)

	// 创建 Lease
	lease := models.ProxyLease{
		ID:              uuid.New().String(),
		ProxyResourceID: proxy.ID,
		GroupID:         groupID,
		StartedAt:       time.Now(),
		IsValid:         true,
	}
	models.DB.Create(&lease)

	spec := models.GroupSpec{
		ID:           groupID,
		NodeID:       req.NodeID,
		ProxyLeaseID: lease.ID,
		TunnelType:   req.TunnelType,
		Apps:         req.Apps,
	}

	runtime := models.GroupRuntime{
		GroupID:      groupID,
		CurrentState: "pending",
	}

	// 生成创建任务
	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "create_group",
		TargetID: groupID,
		Status:   "pending",
	}

	payloadMap := map[string]string{
		"group_id":  groupID,
		"tunnel":    req.TunnelType,
		"proxy_url": proxyURL,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	task := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      req.NodeID,
		Type:        "start_group",
		Payload:     string(payloadBytes),
		Status:      "pending",
		Timeout:     300,
	}

	models.DB.Create(&spec)
	models.DB.Create(&runtime)
	models.DB.Create(&op)
	models.DB.Create(&task)

	c.JSON(http.StatusOK, gin.H{"message": "Proxy group created", "id": groupID, "proxy_url": proxyURL})
}

func replaceProxy(c *gin.Context) {
	groupID := c.Param("id")

	var spec models.GroupSpec
	if err := models.DB.Where("id = ?", groupID).First(&spec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理组不存在"})
		return
	}

	// 查找新的在线可用代理
	var newProxy models.ProxyResource
	if err := models.DB.Where("status = ? AND status != ?", "online", "in_use").First(&newProxy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无可用代理"})
		return
	}

	// 更新旧 Lease 失效并释放旧代理资源
	var oldLease models.ProxyLease
	if err := models.DB.Where("id = ?", spec.ProxyLeaseID).First(&oldLease).Error; err == nil {
		now := time.Now()
		models.DB.Model(&oldLease).Updates(map[string]interface{}{"is_valid": false, "ended_at": &now})
		models.DB.Model(&models.ProxyResource{}).Where("id = ?", oldLease.ProxyResourceID).Update("status", "online")
	}

	// 标记新代理并创建 Lease
	models.DB.Model(&newProxy).Update("status", "in_use")
	newLease := models.ProxyLease{
		ID:              uuid.New().String(),
		ProxyResourceID: newProxy.ID,
		GroupID:         groupID,
		StartedAt:       time.Now(),
		IsValid:         true,
	}
	models.DB.Create(&newLease)
	models.DB.Model(&spec).Update("proxy_lease_id", newLease.ID)

	// 构建代理 URL
	auth := ""
	if newProxy.Username != "" && newProxy.Password != "" {
		auth = fmt.Sprintf("%s:%s@", newProxy.Username, newProxy.Password)
	}
	proxyURL := fmt.Sprintf("%s://%s%s:%d", newProxy.Protocol, auth, newProxy.Host, newProxy.Port)

	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "replace_proxy",
		TargetID: groupID,
		Status:   "pending",
	}

	payloadMap := map[string]string{
		"group_id":  groupID,
		"tunnel":    spec.TunnelType,
		"proxy_url": proxyURL,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	task := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      spec.NodeID,
		Type:        "replace_proxy",
		Payload:     string(payloadBytes),
		Status:      "pending",
		Timeout:     300,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)
	models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "proxy_replacing")

	c.JSON(http.StatusOK, gin.H{"message": "代理替换指令已下发", "proxy_url": proxyURL})
}

func migrateNode(c *gin.Context) {
	groupID := c.Param("id")

	var req struct {
		TargetNodeID string `json:"target_node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var spec models.GroupSpec
	if err := models.DB.Where("id = ?", groupID).First(&spec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理组不存在"})
		return
	}

	oldNodeID := spec.NodeID
	models.DB.Model(&spec).Update("node_id", req.TargetNodeID)

	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "migrate_node",
		TargetID: groupID,
		Status:   "pending",
	}

	// 先在旧节点发起停止，然后在目标节点发起启动 (简单起见我们把任务发给这两个节点，后端依靠状态机后续同步)
	// 在 Node 执行端我们会实现 stop_group
	payloadMap := map[string]string{"group_id": groupID}
	stopBytes, _ := json.Marshal(payloadMap)

	taskStop := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      oldNodeID,
		Type:        "stop_group",
		Payload:     string(stopBytes),
		Status:      "pending",
		Timeout:     300,
	}

	// 新节点的启动需要重新获取 proxyURL
	var lease models.ProxyLease
	var proxy models.ProxyResource
	proxyURL := ""
	if err := models.DB.Where("id = ?", spec.ProxyLeaseID).First(&lease).Error; err == nil {
		if err := models.DB.Where("id = ?", lease.ProxyResourceID).First(&proxy).Error; err == nil {
			auth := ""
			if proxy.Username != "" && proxy.Password != "" {
				auth = fmt.Sprintf("%s:%s@", proxy.Username, proxy.Password)
			}
			proxyURL = fmt.Sprintf("%s://%s%s:%d", proxy.Protocol, auth, proxy.Host, proxy.Port)
		}
	}

	startMap := map[string]string{
		"group_id":  groupID,
		"tunnel":    spec.TunnelType,
		"proxy_url": proxyURL,
	}
	startBytes, _ := json.Marshal(startMap)

	taskStart := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      req.TargetNodeID,
		Type:        "start_group",
		Payload:     string(startBytes),
		Status:      "pending",
		Timeout:     300,
	}

	models.DB.Create(&op)
	models.DB.Create(&taskStop)
	models.DB.Create(&taskStart)
	models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "node_migrating")

	c.JSON(http.StatusOK, gin.H{"message": "节点迁移指令已下发"})
}

func getOperations(c *gin.Context) {
	var ops []models.Operation
	models.DB.Order("created_at desc").Limit(100).Find(&ops)
	c.JSON(http.StatusOK, gin.H{"data": ops})
}

func getTasks(c *gin.Context) {
	var tasks []models.Task
	models.DB.Order("created_at desc").Limit(200).Find(&tasks)
	c.JSON(http.StatusOK, gin.H{"data": tasks})
}

func getSystemInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "online",
		"time":   time.Now(),
	})
}

func getRules(c *gin.Context) {
	var rules []models.Rule
	models.DB.Find(&rules)
	c.JSON(http.StatusOK, gin.H{"data": rules})
}

func addRule(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Condition   string `json:"condition" binding:"required"`
		Action      string `json:"action" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule := models.Rule{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Condition:   req.Condition,
		Action:      req.Action,
		IsEnabled:   true,
	}

	models.DB.Create(&rule)
	c.JSON(http.StatusOK, gin.H{"message": "Rule created", "data": rule})
}

func deleteRule(c *gin.Context) {
	id := c.Param("id")
	models.DB.Where("id = ?", id).Delete(&models.Rule{})
	c.JSON(http.StatusOK, gin.H{"message": "Rule deleted"})
}
