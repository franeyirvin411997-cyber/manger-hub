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
	"multi_node_platform/pkg/utils"
)

func SetupGinRouter() *gin.Engine {
	r := gin.Default()
	r.Use(CorsMiddleware())

	r.GET("/api/v1/node-enrollments/:id/install.sh", getNodeEnrollmentInstallScript)

	adminUser := os.Getenv("ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "admin"
	}

	api := r.Group("/api/v1", gin.BasicAuth(gin.Accounts{adminUser: adminPassword}))

	api.GET("/auth", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "authenticated"})
	})

	// 节点
	api.GET("/nodes", getNodes)
	api.GET("/nodes/:id/logs/stream", streamNodeLogs)
	api.PUT("/nodes/:id", updateNode)
	api.DELETE("/nodes/:id", deleteNode)
	api.GET("/node-enrollments", listNodeEnrollmentsHandler)
	api.POST("/node-enrollments", createNodeEnrollment)

	// 代理
	api.GET("/proxies", getProxies)
	api.POST("/proxies", addProxy)
	api.POST("/proxies/csv", uploadProxiesCSV)
	api.DELETE("/proxies/:id", deleteProxy)
	api.POST("/proxies/:id/promote", promoteProxy)
	api.POST("/proxies/:id/demote", demoteProxy)

	// 代理组
	api.GET("/groups", getGroups)
	api.POST("/groups", createGroup)
	api.POST("/groups/:id/replace_proxy", replaceProxy)
	api.POST("/groups/:id/migrate_node", migrateNode)
	api.POST("/groups/:id/start", startGroup)
	api.POST("/groups/:id/stop", stopGroup)
	api.POST("/groups/:id/restart", restartGroup)
	api.DELETE("/groups/:id", deleteGroup)

	// 应用模板
	api.GET("/apps", getAppTemplates)
	api.POST("/apps", addAppTemplate)
	api.PUT("/apps/:id", updateAppTemplate)
	api.DELETE("/apps/:id", deleteAppTemplate)

	// 账号管理
	api.GET("/accounts", getAccounts)
	api.POST("/accounts", addAccount)
	api.GET("/accounts/:id", getAccountDetail)
	api.PUT("/accounts/:id", updateAccount)
	api.DELETE("/accounts/:id", deleteAccount)
	api.GET("/accounts/bindings", getAccountBindings)

	// 浏览器管理
	api.GET("/browsers", getBrowsers)
	api.POST("/browsers", createBrowser)
	api.DELETE("/browsers/:id", deleteBrowser)
	api.POST("/browsers/:id/stop", stopBrowser)
	api.POST("/browsers/:id/bind", bindBrowserAccount)
	api.GET("/browsers/:id/vnc", proxyBrowserVNC)
	api.GET("/browsers/:id/screenshot", getBrowserScreenshot)
	api.POST("/browsers/:id/action", browserExecAction)

	// 规则
	api.GET("/rules", getRules)
	api.POST("/rules", addRule)
	api.DELETE("/rules/:id", deleteRule)

	// 系统配置
	api.GET("/config", getSystemConfigs)
	api.PUT("/config", updateSystemConfig)
	api.PUT("/config/batch", batchUpdateSystemConfigs)

	// AI
	api.POST("/ai/chat", aiChat)
	api.GET("/ai/logs", getAILogs)

	// 审计
	api.GET("/operations", getOperations)
	api.GET("/tasks", getTasks)

	// 系统信息
	api.GET("/info", getSystemInfo)

	r.StaticFile("/download/node", "/usr/local/bin/node")
	return r
}

func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func getProxies(c *gin.Context) {
	poolType := c.Query("pool_type")
	status := c.Query("status")
	var proxies []models.ProxyResource
	q := models.DB
	if poolType != "" {
		q = q.Where("pool_type = ?", poolType)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	q.Find(&proxies)
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
	var existing models.ProxyResource
	if err := models.DB.Where("host = ? AND port = ?", req.Host, req.Port).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该代理地址已存在"})
		return
	}
	proxy := models.ProxyResource{
		ID: uuid.New().String(), Host: req.Host, Port: req.Port,
		Username: req.Username, Password: req.Password, Protocol: req.Protocol,
		PoolType: "observer", Status: "unknown",
	}
	models.DB.Create(&proxy)
	c.JSON(http.StatusOK, gin.H{"message": "代理已添加", "data": proxy})
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
			continue
		}
		if len(record) < 3 {
			continue
		}
		port, _ := strconv.Atoi(record[2])
		host := record[1]
		var existing models.ProxyResource
		if models.DB.Where("host = ? AND port = ?", host, port).First(&existing).Error == nil {
			continue
		}
		proxy := models.ProxyResource{
			ID: uuid.New().String(), Protocol: record[0], Host: host, Port: port,
			PoolType: "observer", Status: "unknown",
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

func deleteProxy(c *gin.Context) {
	id := c.Param("id")
	var proxy models.ProxyResource
	if err := models.DB.Where("id = ?", id).First(&proxy).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理不存在"})
		return
	}
	if proxy.Status == "in_use" {
		c.JSON(http.StatusConflict, gin.H{"error": "代理正在使用中"})
		return
	}
	models.DB.Where("id = ?", id).Delete(&models.ProxyResource{})
	c.JSON(http.StatusOK, gin.H{"message": "代理已删除"})
}

func promoteProxy(c *gin.Context) {
	id := c.Param("id")
	models.DB.Model(&models.ProxyResource{}).Where("id = ?", id).Updates(map[string]interface{}{
		"pool_type": "formal", "status": "online",
	})
	c.JSON(http.StatusOK, gin.H{"message": "已晋升到正式池"})
}

func demoteProxy(c *gin.Context) {
	id := c.Param("id")
	models.DB.Model(&models.ProxyResource{}).Where("id = ?", id).Updates(map[string]interface{}{
		"pool_type": "observer", "status": "unknown",
	})
	c.JSON(http.StatusOK, gin.H{"message": "已降级到观察池"})
}

// ============================================================
// 代理组 — 修复事务 + 竞态 + 状态联查
// ============================================================

func getGroups(c *gin.Context) {
	var specs []models.GroupSpec
	models.DB.Find(&specs)
	var result []map[string]interface{}
	for _, s := range specs {
		item := map[string]interface{}{
			"id": s.ID, "display_name": s.DisplayName, "node_id": s.NodeID,
			"tunnel_type": s.TunnelType, "apps": s.Apps, "account_bindings": s.AccountBindings,
			"proxy_resource_id": s.ProxyResourceID, "created_at": s.CreatedAt,
		}
		// 联查运行态
		var rt models.GroupRuntime
		if models.DB.Where("group_id = ?", s.ID).First(&rt).Error == nil {
			item["current_state"] = rt.CurrentState
			item["last_error"] = rt.LastError
			item["last_observed_at"] = rt.LastObservedAt
		}
		// 联查代理信息
		if s.ProxyResourceID != "" {
			var proxy models.ProxyResource
			if models.DB.Where("id = ?", s.ProxyResourceID).First(&proxy).Error == nil {
				item["proxy_host"] = proxy.Host
				item["proxy_port"] = proxy.Port
				item["proxy_protocol"] = proxy.Protocol
			}
		}
		result = append(result, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func createGroup(c *gin.Context) {
	var req struct {
		DisplayName     string            `json:"display_name"`
		NodeID          string            `json:"node_id" binding:"required"`
		TunnelType      string            `json:"tunnel_type"`
		ProxyID         string            `json:"proxy_id"`
		Apps            string            `json:"apps"`
		AppConfigs      string            `json:"app_configs"`
		AccountBindings map[string]string `json:"account_bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TunnelType == "" {
		req.TunnelType = models.GetConfig("default_tunnel_type")
		if req.TunnelType == "" {
			req.TunnelType = "tun2socks"
		}
	}

	tx := models.DB.Begin()

	// 代理分配 — 事务内带行锁
	var proxy models.ProxyResource
	if req.ProxyID != "" {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ? AND status = ?", req.ProxyID, "online").First(&proxy).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": "指定代理不可用"})
			return
		}
	} else {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("status = ? AND pool_type = ?", "online", "formal").First(&proxy).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": "无可用代理"})
			return
		}
	}
	tx.Model(&proxy).Update("status", "in_use")

	// 构建代理 URL
	proxyURL := buildProxyURL(proxy)

	groupID := uuid.New().String()

	// Lease
	lease := models.ProxyLease{
		ID: uuid.New().String(), ProxyResourceID: proxy.ID, GroupID: groupID,
		StartedAt: time.Now(), IsValid: true,
	}
	tx.Create(&lease)

	// 账号绑定 JSON
	accountBindingsJSON := "{}"
	if req.AccountBindings != nil {
		b, _ := json.Marshal(req.AccountBindings)
		accountBindingsJSON = string(b)
	}

	spec := models.GroupSpec{
		ID: groupID, DisplayName: req.DisplayName, NodeID: req.NodeID,
		ProxyLeaseID: lease.ID, ProxyResourceID: proxy.ID,
		TunnelType: req.TunnelType, Apps: req.Apps, AppConfigs: req.AppConfigs,
		AccountBindings: accountBindingsJSON,
	}
	runtime := models.GroupRuntime{GroupID: groupID, CurrentState: "pending"}

	// 解析应用命令
	var appList []string
	json.Unmarshal([]byte(req.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(req.AppConfigs), &appConfigs)
	resolvedApps := resolveAppCommandsV2(appList, appConfigs, req.AccountBindings, groupID)
	resolvedAppsBytes, _ := json.Marshal(resolvedApps)

	payloadMap := map[string]string{
		"group_id": groupID, "tunnel_type": req.TunnelType, "proxy_url": proxyURL,
		"resolved_apps": string(resolvedAppsBytes), "apps": req.Apps,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	op := models.Operation{ID: uuid.New().String(), Type: "create_group", TargetID: groupID, Status: "pending", Operator: "admin"}
	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: req.NodeID,
		Type: "start_group", Payload: string(payloadBytes), Status: "pending",
		ScopeKey: fmt.Sprintf("group:%s:start", groupID), Timeout: 300,
	}

	// 创建账号绑定记录
	for appID, accountID := range req.AccountBindings {
		binding := models.AccountGroupBinding{
			ID: uuid.New().String(), AccountID: accountID, GroupID: groupID,
			AppIdentifier: appID, Status: "bound", CreatedAt: time.Now(),
		}
		tx.Create(&binding)
	}

	tx.Create(&spec)
	tx.Create(&runtime)
	tx.Create(&op)
	tx.Create(&task)

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "代理组已创建", "id": groupID, "proxy_url": proxyURL})
}

func replaceProxy(c *gin.Context) {
	groupID := c.Param("id")
	var spec models.GroupSpec
	if err := models.DB.Where("id = ?", groupID).First(&spec).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代理组不存在"})
		return
	}

	tx := models.DB.Begin()

	var newProxy models.ProxyResource
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("status = ? AND pool_type = ?", "online", "formal").First(&newProxy).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "无可用代理"})
		return
	}

	// 释放旧代理
	var oldLease models.ProxyLease
	if tx.Where("id = ?", spec.ProxyLeaseID).First(&oldLease).Error == nil {
		now := time.Now()
		tx.Model(&oldLease).Updates(map[string]interface{}{"is_valid": false, "ended_at": &now})
		tx.Model(&models.ProxyResource{}).Where("id = ?", oldLease.ProxyResourceID).Update("status", "online")
	}

	tx.Model(&newProxy).Update("status", "in_use")
	newLease := models.ProxyLease{
		ID: uuid.New().String(), ProxyResourceID: newProxy.ID, GroupID: groupID,
		StartedAt: time.Now(), IsValid: true,
	}
	tx.Create(&newLease)
	tx.Model(&spec).Updates(map[string]interface{}{"proxy_lease_id": newLease.ID, "proxy_resource_id": newProxy.ID})

	proxyURL := buildProxyURL(newProxy)

	var appList []string
	json.Unmarshal([]byte(spec.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(spec.AppConfigs), &appConfigs)
	var bindings map[string]string
	json.Unmarshal([]byte(spec.AccountBindings), &bindings)
	resolvedApps := resolveAppCommandsV2(appList, appConfigs, bindings, groupID)
	resolvedAppsBytes, _ := json.Marshal(resolvedApps)

	payloadMap := map[string]string{
		"group_id": groupID, "tunnel_type": spec.TunnelType, "proxy_url": proxyURL,
		"resolved_apps": string(resolvedAppsBytes), "apps": spec.Apps,
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	op := models.Operation{ID: uuid.New().String(), Type: "replace_proxy", TargetID: groupID, Status: "pending", Operator: "admin"}
	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID,
		Type: "replace_proxy", Payload: string(payloadBytes), Status: "pending", Timeout: 300,
	}
	tx.Create(&op)
	tx.Create(&task)
	tx.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "proxy_replacing")

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "操作失败"})
		return
	}
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

	op := models.Operation{ID: uuid.New().String(), Type: "migrate_node", TargetID: groupID, Status: "pending", Operator: "admin"}

	stopPayload, _ := json.Marshal(map[string]string{"group_id": groupID, "apps": spec.Apps})
	taskStop := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: oldNodeID,
		Type: "stop_group", Payload: string(stopPayload), Status: "pending", Timeout: 300,
	}

	proxyURL := ""
	var lease models.ProxyLease
	var proxy models.ProxyResource
	if models.DB.Where("id = ?", spec.ProxyLeaseID).First(&lease).Error == nil {
		if models.DB.Where("id = ?", lease.ProxyResourceID).First(&proxy).Error == nil {
			proxyURL = buildProxyURL(proxy)
		}
	}

	var appList []string
	json.Unmarshal([]byte(spec.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(spec.AppConfigs), &appConfigs)
	var bindings map[string]string
	json.Unmarshal([]byte(spec.AccountBindings), &bindings)
	resolvedApps := resolveAppCommandsV2(appList, appConfigs, bindings, groupID)
	resolvedAppsBytes, _ := json.Marshal(resolvedApps)

	startPayload, _ := json.Marshal(map[string]string{
		"group_id": groupID, "tunnel_type": spec.TunnelType, "proxy_url": proxyURL,
		"resolved_apps": string(resolvedAppsBytes), "apps": spec.Apps,
	})
	taskStart := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: req.TargetNodeID,
		Type: "start_group", Payload: string(startPayload), Status: "pending", Timeout: 300,
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
	proxyURL := getGroupProxyURL(spec)
	var appList []string
	json.Unmarshal([]byte(spec.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(spec.AppConfigs), &appConfigs)
	var bindings map[string]string
	json.Unmarshal([]byte(spec.AccountBindings), &bindings)
	resolvedApps := resolveAppCommandsV2(appList, appConfigs, bindings, groupID)
	resolvedAppsBytes, _ := json.Marshal(resolvedApps)

	payloadMap := map[string]string{
		"group_id": groupID, "tunnel_type": spec.TunnelType, "proxy_url": proxyURL,
		"resolved_apps": string(resolvedAppsBytes), "apps": spec.Apps,
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	op := models.Operation{ID: uuid.New().String(), Type: "start_group", TargetID: groupID, Status: "pending", Operator: "admin"}
	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID,
		Type: "start_group", Payload: string(payloadBytes), Status: "pending", Timeout: 300,
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
	payloadBytes, _ := json.Marshal(map[string]string{"group_id": groupID, "apps": spec.Apps})
	op := models.Operation{ID: uuid.New().String(), Type: "stop_group", TargetID: groupID, Status: "pending", Operator: "admin"}
	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID,
		Type: "stop_group", Payload: string(payloadBytes), Status: "pending", Timeout: 300,
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
	payloadBytes, _ := json.Marshal(map[string]string{"group_id": groupID, "apps": spec.Apps})
	op := models.Operation{ID: uuid.New().String(), Type: "restart_group", TargetID: groupID, Status: "pending", Operator: "admin"}
	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID,
		Type: "restart_group", Payload: string(payloadBytes), Status: "pending", Timeout: 300,
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

	tx := models.DB.Begin()

	payloadBytes, _ := json.Marshal(map[string]string{"group_id": groupID, "apps": spec.Apps})
	op := models.Operation{ID: uuid.New().String(), Type: "delete_group", TargetID: groupID, Status: "pending", Operator: "admin"}
	task := models.Task{
		ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID,
		Type: "stop_group", Payload: string(payloadBytes), Status: "pending", Timeout: 300,
	}
	tx.Create(&op)
	tx.Create(&task)

	// 释放代理
	var lease models.ProxyLease
	if tx.Where("id = ?", spec.ProxyLeaseID).First(&lease).Error == nil {
		now := time.Now()
		tx.Model(&lease).Updates(map[string]interface{}{"is_valid": false, "ended_at": &now})
		tx.Model(&models.ProxyResource{}).Where("id = ?", lease.ProxyResourceID).Update("status", "online")
	}

	// 解绑账号
	tx.Model(&models.AccountGroupBinding{}).Where("group_id = ?", groupID).Update("status", "unbound")

	tx.Where("group_id = ?", groupID).Delete(&models.GroupRuntime{})
	tx.Where("id = ?", groupID).Delete(&models.GroupSpec{})
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"message": "代理组已删除"})
}

// ============================================================
// 应用模板
// ============================================================

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
		ID: uuid.New().String(), Identifier: req.Identifier, DisplayName: req.DisplayName,
		DefaultImage: req.DefaultImage, SupportedConfigs: req.SupportedConfigs,
		CommandTemplate: req.CommandTemplate, DriverType: "docker",
	}
	if req.DriverType != "" {
		tmpl.DriverType = req.DriverType
	}
	if err := models.DB.Create(&tmpl).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "标识符已存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "模板已创建", "data": tmpl})
}

func updateAppTemplate(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		DisplayName      *string `json:"display_name"`
		DefaultImage     *string `json:"default_image"`
		SupportedConfigs *string `json:"supported_configs"`
		CommandTemplate  *string `json:"command_template"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]interface{}{}
	if req.DisplayName != nil { updates["display_name"] = *req.DisplayName }
	if req.DefaultImage != nil { updates["default_image"] = *req.DefaultImage }
	if req.SupportedConfigs != nil { updates["supported_configs"] = *req.SupportedConfigs }
	if req.CommandTemplate != nil { updates["command_template"] = *req.CommandTemplate }
	models.DB.Model(&models.AppTemplate{}).Where("id = ?", id).Updates(updates)
	c.JSON(http.StatusOK, gin.H{"message": "模板已更新"})
}

func deleteAppTemplate(c *gin.Context) {
	id := c.Param("id")
	models.DB.Where("id = ?", id).Delete(&models.AppTemplate{})
	c.JSON(http.StatusOK, gin.H{"message": "模板已删除"})
}

// ============================================================
// 规则 / 审计 / 系统信息
// ============================================================

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
		ID: uuid.New().String(), Name: req.Name, Description: req.Description,
		Condition: req.Condition, Action: req.Action, IsEnabled: true,
	}
	models.DB.Create(&rule)
	c.JSON(http.StatusOK, gin.H{"message": "规则已创建", "data": rule})
}

func deleteRule(c *gin.Context) {
	id := c.Param("id")
	models.DB.Where("id = ?", id).Delete(&models.Rule{})
	c.JSON(http.StatusOK, gin.H{"message": "规则已删除"})
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
	var nodesOn, nodesAll int64
	var proxyFormal, proxyObserver, proxyUse int64
	var grpRun, grpErr, grpAll int64
	var accActive, accBan int64
	var brwRun int64
	var taskPend, taskFailed24h int64

	models.DB.Model(&models.Node{}).Count(&nodesAll)
	models.DB.Model(&models.Node{}).Where("online_state = ?", "online").Count(&nodesOn)
	models.DB.Model(&models.ProxyResource{}).Where("pool_type = ?", "formal").Count(&proxyFormal)
	models.DB.Model(&models.ProxyResource{}).Where("pool_type = ?", "observer").Count(&proxyObserver)
	models.DB.Model(&models.ProxyResource{}).Where("status = ?", "in_use").Count(&proxyUse)
	models.DB.Model(&models.GroupRuntime{}).Count(&grpAll)
	models.DB.Model(&models.GroupRuntime{}).Where("current_state = ?", "running").Count(&grpRun)
	models.DB.Model(&models.GroupRuntime{}).Where("current_state = ?", "error").Count(&grpErr)
	models.DB.Model(&models.AppAccount{}).Where("status = ?", "active").Count(&accActive)
	models.DB.Model(&models.AppAccount{}).Where("status = ?", "banned").Count(&accBan)
	models.DB.Model(&models.BrowserInstance{}).Where("status = ?", "running").Count(&brwRun)
	models.DB.Model(&models.Task{}).Where("status = ?", "pending").Count(&taskPend)
	models.DB.Model(&models.Task{}).Where("status = ? AND updated_at > ?", "failed", time.Now().Add(-24*time.Hour)).Count(&taskFailed24h)

	c.JSON(http.StatusOK, gin.H{
		"nodes_online": nodesOn, "nodes_total": nodesAll,
		"proxies_formal": proxyFormal, "proxies_observer": proxyObserver, "proxies_in_use": proxyUse,
		"groups_running": grpRun, "groups_error": grpErr, "groups_total": grpAll,
		"accounts_active": accActive, "accounts_banned": accBan,
		"browsers_running": brwRun,
		"tasks_pending": taskPend, "tasks_failed_24h": taskFailed24h,
		"time": time.Now(),
	})
}

// ============================================================
// 辅助函数
// ============================================================

func buildProxyURL(proxy models.ProxyResource) string {
	auth := ""
	if proxy.Username != "" && proxy.Password != "" {
		auth = fmt.Sprintf("%s:%s@", proxy.Username, proxy.Password)
	}
	return fmt.Sprintf("%s://%s%s:%d", proxy.Protocol, auth, proxy.Host, proxy.Port)
}

func getGroupProxyURL(spec models.GroupSpec) string {
	var lease models.ProxyLease
	var proxy models.ProxyResource
	if models.DB.Where("id = ?", spec.ProxyLeaseID).First(&lease).Error == nil {
		if models.DB.Where("id = ?", lease.ProxyResourceID).First(&proxy).Error == nil {
			return buildProxyURL(proxy)
		}
	}
	return ""
}

// resolveAppCommandsV2 支持 AccountBindings 和旧版 AppConfigs 两种模式
func resolveAppCommandsV2(appList []string, appConfigs map[string]string, accountBindings map[string]string, groupID string) []ResolvedApp {
	mergedConfigs := make(map[string]string)
	for k, v := range appConfigs {
		mergedConfigs[k] = v
	}
	mergedConfigs["device"] = groupID
	mergedConfigs["group_id"] = groupID

	// 从 AccountBindings 解密凭证注入
	for appID, accountID := range accountBindings {
		var account models.AppAccount
		if models.DB.Where("id = ?", accountID).First(&account).Error == nil {
			if creds, err := utils.DecryptJSON(account.Credentials); err == nil {
				for k, v := range creds {
					mergedConfigs[k] = v
					mergedConfigs[appID+"_"+k] = v
				}
			}
		}
	}

	return resolveWithMergedConfigs(appList, mergedConfigs, groupID)
}

func resolveWithMergedConfigs(appList []string, mergedConfigs map[string]string, groupID string) []ResolvedApp {
	var resolved []ResolvedApp
	for _, id := range appList {
		var tmpl models.AppTemplate
		if models.DB.Where("identifier = ?", id).First(&tmpl).Error != nil {
			resolved = append(resolved, ResolvedApp{Identifier: id, RunArgs: []string{"alpine", "sleep", "3600"}})
			continue
		}
		var argsTemplate []string
		if err := json.Unmarshal([]byte(tmpl.CommandTemplate), &argsTemplate); err != nil {
			resolved = append(resolved, ResolvedApp{Identifier: id, RunArgs: []string{"alpine", "sleep", "3600"}})
			continue
		}
		var finalArgs []string
		for _, argTmpl := range argsTemplate {
			arg := argTmpl
			for key, val := range mergedConfigs {
				arg = stringReplace(arg, "{{"+key+"}}", val)
				arg = stringReplace(arg, "{{"+id+"_"+key+"}}", val)
			}
			finalArgs = append(finalArgs, arg)
		}
		resolved = append(resolved, ResolvedApp{Identifier: id, RunArgs: finalArgs})
	}
	return resolved
}

func stringReplace(s, old, newStr string) string {
	result := s
	for {
		idx := -1
		for i := 0; i <= len(result)-len(old); i++ {
			if result[i:i+len(old)] == old {
				idx = i
				break
			}
		}
		if idx < 0 {
			return result
		}
		result = result[:idx] + newStr + result[idx+len(old):]
	}
}
