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
		adminGroup.POST("/groups/:id/start", startGroup)
		adminGroup.POST("/groups/:id/stop", stopGroup)
		adminGroup.POST("/groups/:id/restart", restartGroup)
		adminGroup.DELETE("/groups/:id", deleteGroup)

		// 应用模板
		adminGroup.GET("/apps", getAppTemplates)
		adminGroup.POST("/apps", addAppTemplate)
		adminGroup.DELETE("/apps/:id", deleteAppTemplate)

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

	// 去重检测
	var existing models.ProxyResource
	if err := models.DB.Where("host = ? AND port = ?", req.Host, req.Port).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该代理地址已存在资源池中"})
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
		host := record[1]

		// 去重检测
		var existing models.ProxyResource
		if err := models.DB.Where("host = ? AND port = ?", host, port).First(&existing).Error; err == nil {
			continue // 跳过重复记录
		}

		proxy := models.ProxyResource{
			ID:       uuid.New().String(),
			Protocol: record[0],
			Host:     host,
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
		Apps       string `json:"apps"`        // JSON array of app ids
		AppConfigs string `json:"app_configs"` // JSON map of app configs
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
		AppConfigs:   req.AppConfigs,
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

	// 解析应用清单和配置，转换为 Node 直接可执行的指令结构
	var appList []string
	json.Unmarshal([]byte(req.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(req.AppConfigs), &appConfigs)

	resolvedApps, _ := resolveAppCommands(appList, appConfigs, groupID)
	resolvedAppsBytes, _ := json.Marshal(resolvedApps)

	payloadMap := map[string]string{
		"group_id":      groupID,
		"tunnel":        req.TunnelType,
		"proxy_url":     proxyURL,
		"resolved_apps": string(resolvedAppsBytes),
		"apps":          req.Apps, // 保留原始引用方便追溯
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

	// 解析应用清单和配置，转换为 Node 直接可执行的指令结构
	var appList []string
	json.Unmarshal([]byte(spec.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(spec.AppConfigs), &appConfigs)

	resolvedApps, _ := resolveAppCommands(appList, appConfigs, groupID)
	resolvedAppsBytes, _ := json.Marshal(resolvedApps)

	payloadMap := map[string]string{
		"group_id":      groupID,
		"tunnel":        spec.TunnelType,
		"proxy_url":     proxyURL,
		"resolved_apps": string(resolvedAppsBytes),
		"apps":          spec.Apps,
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
	payloadMap := map[string]string{"group_id": groupID, "apps": spec.Apps}
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

	// 解析应用清单和配置，转换为 Node 直接可执行的指令结构
	var appListMigrate []string
	json.Unmarshal([]byte(spec.Apps), &appListMigrate)
	var appConfigsMigrate map[string]string
	json.Unmarshal([]byte(spec.AppConfigs), &appConfigsMigrate)

	resolvedAppsMigrate, _ := resolveAppCommands(appListMigrate, appConfigsMigrate, groupID)
	resolvedAppsBytesMigrate, _ := json.Marshal(resolvedAppsMigrate)

	startMap := map[string]string{
		"group_id":      groupID,
		"tunnel":        spec.TunnelType,
		"proxy_url":     proxyURL,
		"resolved_apps": string(resolvedAppsBytesMigrate),
		"apps":          spec.Apps,
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

func startGroup(c *gin.Context) {
	groupID := c.Param("id")

	var spec models.GroupSpec
	if err := models.DB.Where("id = ?", groupID).First(&spec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理组不存在"})
		return
	}

	var proxyURL string
	var lease models.ProxyLease
	var proxy models.ProxyResource
	if err := models.DB.Where("id = ?", spec.ProxyLeaseID).First(&lease).Error; err == nil {
		if err := models.DB.Where("id = ?", lease.ProxyResourceID).First(&proxy).Error; err == nil {
			auth := ""
			if proxy.Username != "" && proxy.Password != "" {
				auth = fmt.Sprintf("%s:%s@", proxy.Username, proxy.Password)
			}
			proxyURL = fmt.Sprintf("%s://%s%s:%d", proxy.Protocol, auth, proxy.Host, proxy.Port)
		}
	}

	var appList []string
	json.Unmarshal([]byte(spec.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(spec.AppConfigs), &appConfigs)
	resolvedApps, _ := resolveAppCommands(appList, appConfigs, groupID)
	resolvedAppsBytes, _ := json.Marshal(resolvedApps)

	payloadMap := map[string]string{
		"group_id":      groupID,
		"tunnel":        spec.TunnelType,
		"proxy_url":     proxyURL,
		"resolved_apps": string(resolvedAppsBytes),
		"apps":          spec.Apps,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "start_group",
		TargetID: groupID,
		Status:   "pending",
	}

	task := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      spec.NodeID,
		Type:        "start_group",
		Payload:     string(payloadBytes),
		Status:      "pending",
		Timeout:     300,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)
	models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "pending")

	c.JSON(http.StatusOK, gin.H{"message": "启动指令已下发"})
}

func stopGroup(c *gin.Context) {
	groupID := c.Param("id")

	var spec models.GroupSpec
	if err := models.DB.Where("id = ?", groupID).First(&spec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理组不存在"})
		return
	}

	payloadMap := map[string]string{"group_id": groupID, "apps": spec.Apps}
	payloadBytes, _ := json.Marshal(payloadMap)

	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "stop_group",
		TargetID: groupID,
		Status:   "pending",
	}

	task := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      spec.NodeID,
		Type:        "stop_group",
		Payload:     string(payloadBytes),
		Status:      "pending",
		Timeout:     300,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)
	models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "stopped")

	c.JSON(http.StatusOK, gin.H{"message": "停止指令已下发"})
}

func restartGroup(c *gin.Context) {
	groupID := c.Param("id")

	var spec models.GroupSpec
	if err := models.DB.Where("id = ?", groupID).First(&spec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理组不存在"})
		return
	}

	payloadMap := map[string]string{"group_id": groupID, "apps": spec.Apps}
	payloadBytes, _ := json.Marshal(payloadMap)

	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "restart_group",
		TargetID: groupID,
		Status:   "pending",
	}

	task := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      spec.NodeID,
		Type:        "restart_group",
		Payload:     string(payloadBytes),
		Status:      "pending",
		Timeout:     300,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)
	models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "pending")

	c.JSON(http.StatusOK, gin.H{"message": "重启指令已下发"})
}

func deleteGroup(c *gin.Context) {
	groupID := c.Param("id")

	var spec models.GroupSpec
	if err := models.DB.Where("id = ?", groupID).First(&spec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理组不存在"})
		return
	}

	// 1. 发送停止任务给节点
	payloadMap := map[string]string{"group_id": groupID, "apps": spec.Apps}
	payloadBytes, _ := json.Marshal(payloadMap)

	op := models.Operation{
		ID:       uuid.New().String(),
		Type:     "delete_group",
		TargetID: groupID,
		Status:   "pending",
	}

	task := models.Task{
		ID:          uuid.New().String(),
		OperationID: op.ID,
		NodeID:      spec.NodeID,
		Type:        "stop_group",
		Payload:     string(payloadBytes),
		Status:      "pending",
		Timeout:     300,
	}

	models.DB.Create(&op)
	models.DB.Create(&task)

	// 2. 释放代理
	var lease models.ProxyLease
	if err := models.DB.Where("id = ?", spec.ProxyLeaseID).First(&lease).Error; err == nil {
		now := time.Now()
		models.DB.Model(&lease).Updates(map[string]interface{}{"is_valid": false, "ended_at": &now})
		models.DB.Model(&models.ProxyResource{}).Where("id = ?", lease.ProxyResourceID).Update("status", "online")
	}

	// 3. 删除数据库记录
	models.DB.Where("group_id = ?", groupID).Delete(&models.GroupRuntime{})
	models.DB.Where("id = ?", groupID).Delete(&models.GroupSpec{})

	c.JSON(http.StatusOK, gin.H{"message": "代理组已删除并释放代理资源"})
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

func getAppTemplates(c *gin.Context) {
	var apps []models.AppTemplate
	models.DB.Find(&apps)
	c.JSON(http.StatusOK, gin.H{"data": apps})
}

func addAppTemplate(c *gin.Context) {
	var req struct {
		Identifier       string `json:"identifier" binding:"required"`
		DisplayName      string `json:"display_name" binding:"required"`
		DefaultImage     string `json:"default_image" binding:"required"`
		SupportedConfigs string `json:"supported_configs" binding:"required"`
		CommandTemplate  string `json:"command_template" binding:"required"`
		DriverType       string `json:"driver_type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tmpl := models.AppTemplate{
		ID:               uuid.New().String(),
		Identifier:       req.Identifier,
		DisplayName:      req.DisplayName,
		DefaultImage:     req.DefaultImage,
		SupportedConfigs: req.SupportedConfigs,
		CommandTemplate:  req.CommandTemplate,
		DriverType:       "docker",
	}

	if req.DriverType != "" {
		tmpl.DriverType = req.DriverType
	}

	if err := models.DB.Create(&tmpl).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "应用模板标识符必须唯一"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "App Template created", "data": tmpl})
}

func deleteAppTemplate(c *gin.Context) {
	id := c.Param("id")
	models.DB.Where("id = ?", id).Delete(&models.AppTemplate{})
	c.JSON(http.StatusOK, gin.H{"message": "App Template deleted"})
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
