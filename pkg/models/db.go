package models

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// InitDB 初始化并连接 PostgreSQL 数据库
func InitDB(dsn string) {
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("无法连接到数据库: %v", err)
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

	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Shanghai",
		host, user, password, dbname, port)
}
