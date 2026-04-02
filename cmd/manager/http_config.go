package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"multi_node_platform/pkg/models"
)

func getSystemConfigs(c *gin.Context) {
	category := c.Query("category")
	var configs []models.SystemConfig
	q := models.DB
	if category != "" {
		q = q.Where("category = ?", category)
	}
	q.Order("category, key").Find(&configs)
	for i := range configs {
		if configs[i].Key == "ai_api_key" || configs[i].Key == "tgbot_token" || configs[i].Key == "browser_vnc_password" {
			v := configs[i].Value
			if len(v) > 8 {
				configs[i].Value = v[:4] + "****" + v[len(v)-4:]
			} else if v != "" {
				configs[i].Value = "****"
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": configs})
}

func updateSystemConfig(c *gin.Context) {
	var req struct {
		Key   string `json:"key" binding:"required"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result := models.DB.Model(&models.SystemConfig{}).Where("key = ?", req.Key).Update("value", req.Value)
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "配置项不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "配置已更新"})
}

func batchUpdateSystemConfigs(c *gin.Context) {
	var req struct {
		Configs map[string]string `json:"configs" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tx := models.DB.Begin()
	for key, value := range req.Configs {
		if err := tx.Model(&models.SystemConfig{}).Where("key = ?", key).Update("value", value).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + key})
			return
		}
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "配置批量更新完成"})
}
