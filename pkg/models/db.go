package models

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dsn string) {
	var err error
	maxRetries := 20
	for i := 0; i < maxRetries; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("数据库连接重试 (%d/%d): %v", i+1, maxRetries, err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	err = DB.AutoMigrate(
		&Node{},
		&NodeEnrollment{},
		&ProxyResource{},
		&ProxyLease{},
		&GroupSpec{},
		&GroupRuntime{},
		&AppTemplate{},
		&AppAccount{},
		&AccountGroupBinding{},
		&BrowserInstance{},
		&Operation{},
		&Task{},
		&Rule{},
		&SystemConfig{},
		&AIActionLog{},
	)
	if err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	log.Println("数据库初始化完成")
	seedAppTemplates()
	seedSystemConfigs()
}

func seedAppTemplates() {
	templates := []AppTemplate{
		{
			ID: "app-traffmonetizer", Identifier: "traffmonetizer", DisplayName: "Traffmonetizer",
			DefaultImage: "traffmonetizer/cli_v2:latest", SupportedConfigs: `["token"]`,
			CommandTemplate: `["traffmonetizer/cli_v2:latest", "start", "accept", "--token", "{{token}}"]`,
			DriverType:      "docker",
		},
		{
			ID: "app-repocket", Identifier: "repocket", DisplayName: "Repocket",
			DefaultImage: "repocket/repocket:latest", SupportedConfigs: `["email", "api_key"]`,
			CommandTemplate: `["-e", "RP_EMAIL={{email}}", "-e", "RP_API_KEY={{api_key}}", "repocket/repocket:latest"]`,
			DriverType:      "docker",
		},
		{
			ID: "app-honeygain", Identifier: "honeygain", DisplayName: "Honeygain",
			DefaultImage: "honeygain/honeygain:latest", SupportedConfigs: `["email", "password", "device"]`,
			CommandTemplate: `["honeygain/honeygain:latest", "-tou-accept", "-email", "{{email}}", "-pass", "{{password}}", "-device", "{{device}}"]`,
			DriverType:      "docker",
		},
		{
			ID: "app-packetstream", Identifier: "packetstream", DisplayName: "PacketStream",
			DefaultImage: "packetstream/psclient:latest", SupportedConfigs: `["cid"]`,
			CommandTemplate: `["-e", "CID={{cid}}", "packetstream/psclient:latest"]`,
			DriverType:      "docker",
		},
	}
	for _, t := range templates {
		var existing AppTemplate
		if DB.Where("identifier = ?", t.Identifier).First(&existing).Error != nil {
			DB.Create(&t)
		} else if existing.CommandTemplate == "" {
			DB.Model(&existing).Updates(map[string]interface{}{
				"supported_configs": t.SupportedConfigs,
				"command_template":  t.CommandTemplate,
			})
		}
	}
}

func seedSystemConfigs() {
	defaults := []SystemConfig{
		{Key: "heartbeat_interval_sec", Value: "10", Description: "节点心跳间隔(秒)", Category: "general"},
		{Key: "drift_check_interval_sec", Value: "30", Description: "漂移检测间隔(秒)", Category: "general"},
		{Key: "node_offline_threshold_sec", Value: "180", Description: "节点离线判定阈值(秒)", Category: "general"},
		{Key: "data_retention_days", Value: "7", Description: "操作/任务记录保留天数", Category: "general"},
		{Key: "proxy_check_interval_sec", Value: "60", Description: "代理存活检测间隔(秒)", Category: "proxy"},
		{Key: "default_tunnel_type", Value: "tun2socks", Description: "默认隧道类型", Category: "proxy"},
		{Key: "tun2socks_image", Value: "xjasonlyu/tun2socks:v2.6.0", Description: "tun2socks 镜像", Category: "proxy"},
		{Key: "tun2proxy_image", Value: "ghcr.io/blechschmidt/tun2proxy:latest", Description: "tun2proxy 镜像", Category: "proxy"},
		{Key: "browser_image", Value: "kasmweb/chromium:1.16.1", Description: "默认浏览器镜像", Category: "browser"},
		{Key: "browser_vnc_password", Value: "changeme", Description: "浏览器 VNC 默认密码", Category: "browser"},
		{Key: "tgbot_token", Value: "", Description: "Telegram Bot Token", Category: "tgbot"},
		{Key: "tgbot_admin_ids", Value: "[]", Description: "允许操作的 Telegram 用户 ID 列表 (JSON)", Category: "tgbot"},
		{Key: "ai_provider", Value: "openai", Description: "AI 提供商: openai, anthropic, xai", Category: "ai"},
		{Key: "ai_api_key", Value: "", Description: "AI API Key", Category: "ai"},
		{Key: "ai_model", Value: "gpt-4o", Description: "AI 模型名称", Category: "ai"},
		{Key: "ai_base_url", Value: "https://api.openai.com/v1", Description: "AI API Base URL", Category: "ai"},
		{Key: "webhook_url", Value: "", Description: "告警 Webhook URL", Category: "general"},
	}
	for _, c := range defaults {
		var existing SystemConfig
		if DB.Where("key = ?", c.Key).First(&existing).Error != nil {
			DB.Create(&c)
		}
	}
}

// GetConfig 获取系统配置值
func GetConfig(key string) string {
	var c SystemConfig
	if DB.Where("key = ?", key).First(&c).Error == nil {
		return c.Value
	}
	return ""
}

// SetConfig 设置系统配置值
func SetConfig(key, value string) {
	DB.Model(&SystemConfig{}).Where("key = ?", key).Update("value", value)
}

func GetDSNFromEnv() string {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		password = "password"
	}
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "platform_db"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		host, user, password, dbname, port)
}
