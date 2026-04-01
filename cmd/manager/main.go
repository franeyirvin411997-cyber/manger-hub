package main

import (
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"
	"multi_node_platform/pkg/models"
	"multi_node_platform/pkg/workers"
	pb "multi_node_platform/api/pb"
)

func main() {
	log.Println("Initializing Manager Backend...")

	// 初始化数据库
	dsn := models.GetDSNFromEnv()
	models.InitDB(dsn)

	// 启动后台工作协程
	go workers.StartDriftChecker()
	go workers.StartProxyLifecycleChecker()
	go workers.StartRuleEngine()
	go workers.StartDataCleaner()

	// 启动 gRPC 服务用于与节点通信
	go startGRPCServer(":50051")

	// 启动 Gin HTTP 服务用于提供 Web 后台 API
	r := SetupGinRouter()
	log.Println("Starting REST API server on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("无法启动 HTTP 服务器: %v", err)
	}
}

func startGRPCServer(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()

	// 注册 platform.NodeServiceServer
	pb.RegisterNodeServiceServer(s, &GrpcServer{})

	log.Printf("Starting gRPC server on %s", addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
