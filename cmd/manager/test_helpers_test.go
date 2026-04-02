package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"multi_node_platform/pkg/models"
)

func setupManagerTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	models.DB = db
	if err := models.DB.AutoMigrate(&models.Node{}, &models.NodeEnrollment{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "admin")
	return SetupGinRouter()
}
