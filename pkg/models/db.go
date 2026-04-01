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

// InitDB 初始化并连接 PostgreSQL 数据库
func InitDB(dsn string) {
	var err error
	maxRetries := 20
	for i := 0; i < maxRetries; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("无法连接到数据库 (重试 %d/%d): %v", i+1, maxRetries, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatalf("无法连接到数据库，已达到最大重试次数: %v", err)
	}

	// 自动迁移模式，确保数据表结构与模型定义一致
	err = DB.AutoMigrate(
		&Node{},
		&ProxyResource{},
		&ProxyLease{},
		&GroupSpec{},
		&GroupRuntime{},
		&AppTemplate{},
		&Operation{},
		&Task{},
		&Rule{},
	)
	if err != nil {
		log.Fatalf("自动迁移数据库失败: %v", err)
	}

	fmt.Println("数据库初始化成功，并已完成自动迁移。")
	seedAppTemplates()
}

func seedAppTemplates() {
	templates := []AppTemplate{
		{
			ID:               "app-traffmonetizer",
			Identifier:       "traffmonetizer",
			DisplayName:      "Traffmonetizer",
			DefaultImage:     "traffmonetizer/cli_v2:latest",
			SupportedConfigs: `{"token": "string"}`, // 用户输入 token
			DriverType:       "docker",
		},
		{
			ID:               "app-repocket",
			Identifier:       "repocket",
			DisplayName:      "Repocket",
			DefaultImage:     "repocket/repocket:latest",
			SupportedConfigs: `{"email": "string", "api_key": "string"}`,
			DriverType:       "docker",
		},
		{
			ID:               "app-honeygain",
			Identifier:       "honeygain",
			DisplayName:      "Honeygain",
			DefaultImage:     "honeygain/honeygain:latest",
			SupportedConfigs: `{"email": "string", "password": "string"}`,
			DriverType:       "docker",
		},
		{
			ID:               "app-packetstream",
			Identifier:       "packetstream",
			DisplayName:      "PacketStream",
			DefaultImage:     "packetstream/psclient:latest",
			SupportedConfigs: `{"cid": "string"}`,
			DriverType:       "docker",
		},
	}

	for _, t := range templates {
		var existing AppTemplate
		if DB.Where("identifier = ?", t.Identifier).First(&existing).Error != nil {
			DB.Create(&t)
		}
	}
}

// GetDSNFromEnv 从环境变量中获取数据库连接字符串
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
