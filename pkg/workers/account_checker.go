package workers

import (
	"log"
	"time"

	"multi_node_platform/pkg/models"
)

func StartAccountStatusChecker() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		var browsers []models.BrowserInstance
		models.DB.Where("status = ? AND account_id != ''", "running").Find(&browsers)
		for _, b := range browsers {
			var account models.AppAccount
			if models.DB.Where("id = ?", b.AccountID).First(&account).Error != nil {
				continue
			}
			log.Printf("[AccountChecker] 账号 %s (%s) 待自动化检查 via 浏览器 %s", account.DisplayName, account.AppIdentifier, b.ID)
		}
	}
}
