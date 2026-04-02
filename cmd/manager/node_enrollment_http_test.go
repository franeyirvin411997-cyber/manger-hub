package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multi_node_platform/pkg/models"
)

func TestCreateAndListNodeEnrollment(t *testing.T) {
	router := setupManagerTestRouter(t)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/node-enrollments",
		strings.NewReader(`{"display_name":"tokyo-01","manager_http_addr":"https://manager.example.com","manager_grpc_addr":"manager.example.com:50051"}`),
	)
	createReq.SetBasicAuth("admin", "admin")
	createReq.Header.Set("Content-Type", "application/json")

	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data struct {
			ID             string `json:"id"`
			InstallCommand string `json:"install_command"`
			ExpiresAt      string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.Data.ID == "" {
		t.Fatal("expected enrollment id")
	}
	if !strings.Contains(created.Data.InstallCommand, "/api/v1/node-enrollments/"+created.Data.ID+"/install.sh?token=") {
		t.Fatalf("unexpected install command: %s", created.Data.InstallCommand)
	}
	if strings.Contains(created.Data.InstallCommand, "sudo bash") {
		t.Fatalf("install command should not require sudo: %s", created.Data.InstallCommand)
	}
	if created.Data.ExpiresAt == "" {
		t.Fatal("expected expires_at")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-enrollments", nil)
	listReq.SetBasicAuth("admin", "admin")
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}

	var listed struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			Status      string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listed.Data) != 1 {
		t.Fatalf("expected 1 enrollment, got %d", len(listed.Data))
	}
	if listed.Data[0].DisplayName != "tokyo-01" {
		t.Fatalf("display_name = %s", listed.Data[0].DisplayName)
	}
	if listed.Data[0].Status != "pending" {
		t.Fatalf("status = %s", listed.Data[0].Status)
	}
}

func TestGetNodeEnrollmentInstallScript(t *testing.T) {
	router := setupManagerTestRouter(t)

	enrollment := models.NodeEnrollment{
		ID:              "enroll-1",
		DisplayName:     "tokyo-01",
		ManagerHTTPAddr: "https://manager.example.com",
		ManagerGrpcAddr: "manager.example.com:50051",
		TokenHash:       hashEnrollmentToken("secret-token"),
		ExpiresAt:       time.Now().UTC().Add(24 * time.Hour),
	}
	if err := models.DB.Create(&enrollment).Error; err != nil {
		t.Fatalf("seed enrollment: %v", err)
	}

	okReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-enrollments/enroll-1/install.sh?token=secret-token", nil)
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, okReq)

	if okRec.Code != http.StatusOK {
		t.Fatalf("script status = %d, body = %s", okRec.Code, okRec.Body.String())
	}
	body := okRec.Body.String()
	if !strings.Contains(body, "NODE_ENROLLMENT_ID=enroll-1") {
		t.Fatalf("script missing enrollment id: %s", body)
	}
	if !strings.Contains(body, "NODE_ENROLLMENT_TOKEN=secret-token") {
		t.Fatalf("script missing enrollment token: %s", body)
	}
	if !strings.Contains(body, "MANAGER_IP=manager.example.com:50051") {
		t.Fatalf("script missing manager grpc addr: %s", body)
	}
	if !strings.Contains(body, "NODE_TOKEN_FILE=/var/lib/mnp-node/token") {
		t.Fatalf("script missing node token file path: %s", body)
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-enrollments/enroll-1/install.sh?token=wrong-token", nil)
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid token status = %d, body = %s", badRec.Code, badRec.Body.String())
	}
}
