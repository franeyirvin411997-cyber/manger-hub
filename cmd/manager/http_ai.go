package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"multi_node_platform/pkg/ai"
	"multi_node_platform/pkg/models"
)

func aiChat(c *gin.Context) {
	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agent := ai.NewAgent()
	result, err := agent.Chat(req.Message, "web")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func getAILogs(c *gin.Context) {
	var logs []models.AIActionLog
	models.DB.Order("created_at desc").Limit(100).Find(&logs)
	c.JSON(http.StatusOK, gin.H{"data": logs})
}
