package main

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"multi_node_platform/pkg/models"
	"multi_node_platform/pkg/utils"
)

func getAccounts(c *gin.Context) {
	appID := c.Query("app_identifier")
	var accounts []models.AppAccount
	q := models.DB
	if appID != "" {
		q = q.Where("app_identifier = ?", appID)
	}
	q.Find(&accounts)

	// 构建安全视图
	var views []map[string]interface{}
	for _, a := range accounts {
		view := map[string]interface{}{
			"id":             a.ID,
			"app_identifier": a.AppIdentifier,
			"display_name":   a.DisplayName,
			"status":         a.Status,
			"notes":          a.Notes,
			"created_at":     a.CreatedAt,
			"updated_at":     a.UpdatedAt,
		}
		// 解密并掩码显示
		if creds, err := utils.DecryptJSON(a.Credentials); err == nil {
			view["masked_credentials"] = utils.MaskCredentials(creds)
			keys := make([]string, 0, len(creds))
			for k := range creds {
				keys = append(keys, k)
			}
			view["credential_keys"] = keys
		}
		views = append(views, view)
	}
	c.JSON(http.StatusOK, gin.H{"data": views})
}

func getAccountDetail(c *gin.Context) {
	id := c.Param("id")
	var account models.AppAccount
	if err := models.DB.Where("id = ?", id).First(&account).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		return
	}
	view := map[string]interface{}{
		"id":             account.ID,
		"app_identifier": account.AppIdentifier,
		"display_name":   account.DisplayName,
		"status":         account.Status,
		"notes":          account.Notes,
		"created_at":     account.CreatedAt,
	}
	if creds, err := utils.DecryptJSON(account.Credentials); err == nil {
		view["credentials"] = creds // 详情页返回完整凭证
	}
	c.JSON(http.StatusOK, gin.H{"data": view})
}

func addAccount(c *gin.Context) {
	var req struct {
		AppIdentifier string            `json:"app_identifier" binding:"required"`
		DisplayName   string            `json:"display_name" binding:"required"`
		Credentials   map[string]string `json:"credentials" binding:"required"`
		Notes         string            `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证 app_identifier 存在
	var tmpl models.AppTemplate
	if err := models.DB.Where("identifier = ?", req.AppIdentifier).First(&tmpl).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "应用模板不存在: " + req.AppIdentifier})
		return
	}

	// 验证所需配置项齐全
	var supportedKeys []string
	json.Unmarshal([]byte(tmpl.SupportedConfigs), &supportedKeys)
	for _, key := range supportedKeys {
		if _, ok := req.Credentials[key]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必填配置项: " + key})
			return
		}
	}

	encrypted, err := utils.EncryptJSON(req.Credentials)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加密失败"})
		return
	}

	account := models.AppAccount{
		ID:            uuid.New().String(),
		AppIdentifier: req.AppIdentifier,
		DisplayName:   req.DisplayName,
		Credentials:   encrypted,
		Status:        "active",
		Notes:         req.Notes,
	}
	models.DB.Create(&account)
	c.JSON(http.StatusOK, gin.H{"message": "账号已添加", "data": map[string]string{"id": account.ID}})
}

func updateAccount(c *gin.Context) {
	id := c.Param("id")
	var account models.AppAccount
	if err := models.DB.Where("id = ?", id).First(&account).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		return
	}

	var req struct {
		DisplayName *string            `json:"display_name"`
		Credentials map[string]string  `json:"credentials"`
		Status      *string            `json:"status"`
		Notes       *string            `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}
	if req.Credentials != nil {
		encrypted, err := utils.EncryptJSON(req.Credentials)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "加密失败"})
			return
		}
		updates["credentials"] = encrypted
	}

	models.DB.Model(&account).Updates(updates)
	c.JSON(http.StatusOK, gin.H{"message": "账号已更新"})
}

func deleteAccount(c *gin.Context) {
	id := c.Param("id")

	// 检查是否有活跃绑定
	var count int64
	models.DB.Model(&models.AccountGroupBinding{}).Where("account_id = ? AND status = ?", id, "bound").Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该账号仍有活跃部署绑定，请先解绑"})
		return
	}

	models.DB.Where("id = ?", id).Delete(&models.AppAccount{})
	c.JSON(http.StatusOK, gin.H{"message": "账号已删除"})
}

func getAccountBindings(c *gin.Context) {
	accountID := c.Query("account_id")
	groupID := c.Query("group_id")

	q := models.DB.Model(&models.AccountGroupBinding{})
	if accountID != "" {
		q = q.Where("account_id = ?", accountID)
	}
	if groupID != "" {
		q = q.Where("group_id = ?", groupID)
	}

	var bindings []models.AccountGroupBinding
	q.Find(&bindings)
	c.JSON(http.StatusOK, gin.H{"data": bindings})
}
