package main

import (
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"
	"multi_node_platform/pkg/models"
	"multi_node_platform/pkg/tgbot"
	"multi_node_platform/pkg/workers"
	pb "multi_node_platform/api/pb"
)

func main() {
	log.Println("Initializing Manager Backend...")

	dsn := models.GetDSNFromEnv()
	models.InitDB(dsn)

	// 后台 Workers
	go workers.StartDriftChecker()
	go workers.StartProxyLifecycleChecker()
	go workers.StartRuleEngine()
	go workers.StartDataCleaner()
	go workers.StartTaskTimeoutChecker()
	go workers.StartBrowserChecker()
	go workers.StartAccountStatusChecker()

	// Telegram Bot
	go tgbot.StartBot()

	// gRPC
	go startGRPCServer(":50051")

	// HTTP
	r := SetupGinRouter()
	log.Println("Starting REST API server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("HTTP 服务器启动失败: %v", err)
	}
}

func startGRPCServer(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("gRPC listen 失败: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterNodeServiceServer(s, &GrpcServer{})
	log.Printf("Starting gRPC server on %s", addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("gRPC serve 失败: %v", err)
	}
}
