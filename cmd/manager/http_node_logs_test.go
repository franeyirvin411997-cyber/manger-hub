package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"multi_node_platform/pkg/models"
)

func TestNodeLogStreamRejectsOfflineNode(t *testing.T) {
	router := setupManagerTestRouter(t)
	models.DB.Create(&models.Node{
		ID:          "node-offline",
		DisplayName: "offline",
		Hostname:    "offline",
		Token:       "token-offline",
		OnlineState: "offline",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-offline/logs/stream?source_type=node_service", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestNodeLogStreamRejectsInvalidContainerName(t *testing.T) {
	router := setupManagerTestRouter(t)
	models.DB.Create(&models.Node{
		ID:              "node-online",
		DisplayName:     "online",
		Hostname:        "online",
		Token:           "token-online",
		OnlineState:     "online",
		LastHeartbeatAt: time.Now(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-online/logs/stream?source_type=container&container_name=mysql", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
