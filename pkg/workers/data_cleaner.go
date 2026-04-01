package workers

import (
	"log"
	"time"

	"multi_node_platform/pkg/models"
)

// StartDataCleaner 定期清理过期的日志与任务记录 (默认保留 7 天)
func StartDataCleaner() {
	// 每 6 小时执行一次
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for {
		// 启动立即执行一次
		cleanupOldData()
		<-ticker.C
	}
}

func cleanupOldData() {
	// 计算 7 天前的时间
	threshold := time.Now().Add(-7 * 24 * time.Hour)

	resOps := models.DB.Where("created_at < ?", threshold).Delete(&models.Operation{})
	resTasks := models.DB.Where("created_at < ?", threshold).Delete(&models.Task{})

	log.Printf("[DataCleaner] 数据清理完成: 删除了 %d 条操作记录，%d 条任务记录 (阈值: %v)",
		resOps.RowsAffected, resTasks.RowsAffected, threshold.Format("2006-01-02 15:04:05"))
}
