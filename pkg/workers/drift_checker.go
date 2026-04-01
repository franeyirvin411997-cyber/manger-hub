package workers

import (
	"log"
	"time"

	"multi_node_platform/pkg/models"
)

// StartDriftChecker 定期检测节点心跳超时与代理分配状态漂移
func StartDriftChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// 1. 节点离线检测: 超过 3 分钟未上报心跳视为离线
		threshold := time.Now().Add(-3 * time.Minute)
		res := models.DB.Model(&models.Node{}).
			Where("last_heartbeat_at < ? AND online_state = ?", threshold, "online").
			Update("online_state", "offline")

		if res.Error != nil {
			log.Printf("[DriftChecker] 节点离线检测失败: %v", res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("[DriftChecker] 已将 %d 个超时节点标记为离线", res.RowsAffected)
		}

		// 2. 释放无效或孤立的代理分配 (漂移修正)
		// 找出状态为 in_use 但实际已没有对应代理组或组状态为 deleting/error 的分配关系
		var orphanedLeases []models.ProxyLease

		models.DB.Raw(`
			SELECT pl.* FROM proxy_leases pl
			LEFT JOIN group_specs gs ON pl.group_id = gs.id
			LEFT JOIN group_runtimes gr ON pl.group_id = gr.group_id
			WHERE pl.is_valid = true
			  AND (gs.id IS NULL OR gr.current_state IN ('deleting', 'stopped'))
		`).Scan(&orphanedLeases)

		for _, lease := range orphanedLeases {
			now := time.Now()

			// 结束分配关系
			models.DB.Model(&lease).Updates(map[string]interface{}{
				"is_valid": false,
				"ended_at": &now,
			})

			// 释放代理资源本体 (恢复为在线状态供他人分配)
			models.DB.Model(&models.ProxyResource{}).
				Where("id = ?", lease.ProxyResourceID).
				Update("status", "online")

			log.Printf("[DriftChecker] 修正漂移: 已释放代理资源 %s (原属组 %s)", lease.ProxyResourceID, lease.GroupID)
		}
	}
}
