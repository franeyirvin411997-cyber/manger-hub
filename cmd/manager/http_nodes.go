package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"multi_node_platform/pkg/models"
)

func getNodes(c *gin.Context) {
	var nodes []models.Node
	models.DB.Find(&nodes)
	c.JSON(http.StatusOK, gin.H{"data": nodes})
}

func updateNode(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		DisplayName *string `json:"display_name"`
		MaxGroups   *int    `json:"max_groups"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]interface{}{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.MaxGroups != nil {
		updates["max_groups"] = *req.MaxGroups
	}
	models.DB.Model(&models.Node{}).Where("id = ?", id).Updates(updates)
	c.JSON(http.StatusOK, gin.H{"message": "节点已更新"})
}

func deleteNode(c *gin.Context) {
	id := c.Param("id")
	var count int64
	models.DB.Model(&models.GroupSpec{}).Where("node_id = ?", id).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("该节点上还有 %d 个代理组，请先迁移或删除", count)})
		return
	}
	models.DB.Where("id = ?", id).Delete(&models.Node{})
	c.JSON(http.StatusOK, gin.H{"message": "节点已删除"})
}

func createNodeEnrollment(c *gin.Context) {
	var req struct {
		DisplayName     string `json:"display_name"`
		ManagerHTTPAddr string `json:"manager_http_addr" binding:"required"`
		ManagerGrpcAddr string `json:"manager_grpc_addr" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateEnrollmentCreateInput(req.ManagerHTTPAddr, req.ManagerGrpcAddr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enrollment, token, err := newNodeEnrollment(req.DisplayName, req.ManagerHTTPAddr, req.ManagerGrpcAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建接入授权失败"})
		return
	}
	if err := models.DB.Create(&enrollment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存接入授权失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":              enrollment.ID,
		"expires_at":      enrollment.ExpiresAt,
		"install_command": buildInstallCommand(enrollment.ManagerHTTPAddr, enrollment.ID, token),
	}})
}

func listNodeEnrollmentsHandler(c *gin.Context) {
	enrollments, err := listNodeEnrollments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询接入授权失败"})
		return
	}

	now := time.Now().UTC()
	data := make([]gin.H, 0, len(enrollments))
	for _, enrollment := range enrollments {
		data = append(data, gin.H{
			"id":                enrollment.ID,
			"display_name":      enrollment.DisplayName,
			"manager_http_addr": enrollment.ManagerHTTPAddr,
			"manager_grpc_addr": enrollment.ManagerGrpcAddr,
			"expires_at":        enrollment.ExpiresAt,
			"used_at":           enrollment.UsedAt,
			"used_by_node_id":   enrollment.UsedByNodeID,
			"created_at":        enrollment.CreatedAt,
			"status":            nodeEnrollmentStatus(enrollment, now),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func getNodeEnrollmentInstallScript(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 token"})
		return
	}

	enrollment, err := findUsableEnrollment(c.Param("id"), token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.String(http.StatusOK, renderNodeInstallScript(enrollment, token))
}
