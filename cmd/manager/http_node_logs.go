package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"multi_node_platform/pkg/models"
)

func isAllowedContainerName(name string) bool {
	return strings.HasPrefix(name, "tunnel-") || strings.HasPrefix(name, "app-") || strings.HasPrefix(name, "browser-")
}

func validateNodeLogQuery(sourceType, containerName string) error {
	switch sourceType {
	case "node_service":
		return nil
	case "container":
		if containerName == "" {
			return fmt.Errorf("container_name 不能为空")
		}
		if !isAllowedContainerName(containerName) {
			return fmt.Errorf("container_name 不在允许范围")
		}
		return nil
	default:
		return fmt.Errorf("source_type 无效")
	}
}

func streamNodeLogs(c *gin.Context) {
	nodeID := c.Param("id")
	sourceType := c.Query("source_type")
	containerName := c.Query("container_name")

	if err := validateNodeLogQuery(sourceType, containerName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var node models.Node
	if err := models.DB.Where("id = ?", nodeID).First(&node).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
		return
	}
	if node.OnlineState != "online" {
		c.JSON(http.StatusConflict, gin.H{"error": "节点不在线"})
		return
	}

	sub, err := managerNodeLogHub.createSubscription(nodeID, sourceType, containerName)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	defer managerNodeLogHub.cancelSubscription(sub.ID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	notify := c.Request.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-sub.Events:
			if !ok {
				return
			}
			payload, _ := json.Marshal(gin.H{
				"type":      msg.StreamType,
				"content":   msg.Content,
				"timestamp": time.Unix(msg.TimestampUnix, 0).UTC().Format(time.RFC3339),
			})
			fmt.Fprintf(c.Writer, "event: %s\n", msg.StreamType)
			fmt.Fprintf(c.Writer, "data: %s\n\n", payload)
			c.Writer.Flush()
		}
	}
}
