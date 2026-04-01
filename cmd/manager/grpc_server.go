package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	pb "multi_node_platform/api/pb"
	"multi_node_platform/pkg/models"
)

// GrpcServer 实现了 platform.NodeServiceServer
type GrpcServer struct {
	pb.UnimplementedNodeServiceServer
}

// Register 节点注册接口
func (s *GrpcServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	log.Printf("收到节点注册请求: ID=%s, Hostname=%s", req.NodeId, req.Hostname)

	// 生成新 Token
	token := uuid.New().String()

	// 保存或更新节点信息
	node := models.Node{
		ID:              req.NodeId,
		DisplayName:     req.Hostname, // 默认使用 hostname 作为显示名
		Hostname:        req.Hostname,
		Token:           token,
		Capabilities:    fmt.Sprintf("%v", req.Capabilities),
		Version:         req.Version,
		OnlineState:     "online",
		LastHeartbeatAt: time.Now(),
	}

	if err := models.DB.Save(&node).Error; err != nil {
		log.Printf("节点保存失败: %v", err)
		return &pb.RegisterResponse{Success: false, Message: "Internal Server Error"}, nil
	}

	return &pb.RegisterResponse{
		Success: true,
		Token:   token,
		Message: "注册成功",
	}, nil
}

// Heartbeat 节点心跳与状态上报
func (s *GrpcServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	var node models.Node
	if err := models.DB.Where("id = ? AND token = ?", req.NodeId, req.Token).First(&node).Error; err != nil {
		return &pb.HeartbeatResponse{Success: false, Message: "未授权的节点或Token无效"}, nil
	}

	// 更新心跳时间
	node.LastHeartbeatAt = time.Now()
	node.OnlineState = "online"
	models.DB.Save(&node)

	// 处理快照上报 (此处省略复杂的运行态投影逻辑，仅做日志)
	if len(req.Snapshots) > 0 {
		log.Printf("收到节点 %s 的 %d 个快照状态", req.NodeId, len(req.Snapshots))
	}

	// 查询是否有挂起的任务 (简单演示)
	var pendingTasks []models.Task
	models.DB.Where("node_id = ? AND status = ?", req.NodeId, "pending").Find(&pendingTasks)

	var pbTasks []*pb.Task
	for _, t := range pendingTasks {
		pbTasks = append(pbTasks, &pb.Task{
			TaskId:      t.ID,
			OperationId: t.OperationID,
			Type:        t.Type,
			PayloadJson: t.Payload,
		})

		// 更新任务状态为 running
		t.Status = "running"
		models.DB.Save(&t)
	}

	return &pb.HeartbeatResponse{
		Success:      true,
		Message:      "心跳接收成功",
		PendingTasks: pbTasks,
	}, nil
}

// ReportTaskResult 任务结果回传
func (s *GrpcServer) ReportTaskResult(ctx context.Context, req *pb.TaskResult) (*pb.TaskResultResponse, error) {
	log.Printf("收到任务结果: TaskID=%s, Status=%s", req.TaskId, req.Status)

	models.DB.Model(&models.Task{}).Where("id = ?", req.TaskId).Updates(map[string]interface{}{
		"status":        req.Status,
		"error_message": req.ErrorMessage,
		"result":        req.ResultJson,
	})

	return &pb.TaskResultResponse{Success: true}, nil
}
