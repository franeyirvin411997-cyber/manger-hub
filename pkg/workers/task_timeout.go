package workers

import (
	"log"
	"time"

	"multi_node_platform/pkg/models"
)

func StartTaskTimeoutChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		var tasks []models.Task
		models.DB.Where("status = ?", "running").Find(&tasks)
		for _, t := range tasks {
			if t.Timeout > 0 && time.Since(t.UpdatedAt).Seconds() > float64(t.Timeout) {
				models.DB.Model(&t).Updates(map[string]interface{}{
					"status":        "failed",
					"error_message": "执行超时",
				})
				if t.Type == "start_group" || t.Type == "restart_group" || t.Type == "replace_proxy" {
					var payload map[string]string
					if err := parseJSON(t.Payload, &payload); err == nil {
						if gid, ok := payload["group_id"]; ok {
							models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
								"current_state": "error",
								"last_error":    "任务执行超时",
							})
						}
					}
				}
				if t.Type == "start_browser" {
					var payload map[string]string
					if err := parseJSON(t.Payload, &payload); err == nil {
						if bid, ok := payload["browser_id"]; ok {
							models.DB.Model(&models.BrowserInstance{}).Where("id = ?", bid).Updates(map[string]interface{}{
								"status":     "error",
								"last_error": "启动超时",
							})
						}
					}
				}
				log.Printf("[TaskTimeout] 任务 %s 超时标记失败", t.ID)
			}
		}
	}
}
