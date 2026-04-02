package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	pb "multi_node_platform/api/pb"
	"multi_node_platform/pkg/models"
)

// GrpcServer 实现了 platform.NodeServiceServer
type GrpcServer struct {
	pb.UnimplementedNodeServiceServer
}

func (s *GrpcServer) LogStream(stream pb.NodeService_LogStreamServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if first.MessageType != "stream_open" {
		return status.Error(codes.InvalidArgument, "missing stream_open")
	}

	var node models.Node
	if err := models.DB.Where("id = ? AND token = ?", first.NodeId, first.Token).First(&node).Error; err != nil {
		return status.Error(codes.Unauthenticated, "未授权的节点或Token无效")
	}

	managerNodeLogHub.bindNodeStream(first.NodeId, first.Token, &managedNodeLogStream{stream: stream})
	defer managerNodeLogHub.unbindNodeStream(first.NodeId)

	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if msg.SubscriptionId == "" {
			continue
		}
		if err := managerNodeLogHub.publishChunk(msg); err != nil {
			log.Printf("日志块分发失败: %v", err)
		}
	}
}

// Register 节点注册接口
func (s *GrpcServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	log.Printf("收到节点注册: ID=%s, Hostname=%s", req.NodeId, req.Hostname)

	if req.EnrollmentId == "" || req.EnrollmentToken == "" {
		return &pb.RegisterResponse{Success: false, Message: "节点未授权接入"}, nil
	}

	enrollment, err := findUsableEnrollment(req.EnrollmentId, req.EnrollmentToken)
	if err != nil {
		return &pb.RegisterResponse{Success: false, Message: err.Error()}, nil
	}

	token := uuid.New().String()

	// 提取节点 IP（从 gRPC peer 信息）
	nodeIP := ""
	if p, ok := peer.FromContext(ctx); ok {
		addr := p.Addr.String()
		if host, _, err := net.SplitHostPort(addr); err == nil {
			nodeIP = host
		}
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = enrollment.DisplayName
	}
	if displayName == "" {
		displayName = req.Hostname
	}

	node := models.Node{
		ID:              req.NodeId,
		DisplayName:     displayName,
		Hostname:        req.Hostname,
		IP:              nodeIP,
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
	if err := consumeEnrollment(enrollment, req.NodeId); err != nil {
		log.Printf("接入授权标记失败: %v", err)
		return &pb.RegisterResponse{Success: false, Message: "接入授权写入失败"}, nil
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

// ReportTaskResult 任务结果回传 — 联动更新目标实体状态
func (s *GrpcServer) ReportTaskResult(ctx context.Context, req *pb.TaskResult) (*pb.TaskResultResponse, error) {
	log.Printf("收到任务结果: TaskID=%s, Status=%s", req.TaskId, req.Status)

	// 更新 Task
	var task models.Task
	if err := models.DB.Where("id = ?", req.TaskId).First(&task).Error; err != nil {
		return &pb.TaskResultResponse{Success: false}, nil
	}
	models.DB.Model(&task).Updates(map[string]interface{}{
		"status":        req.Status,
		"error_message": req.ErrorMessage,
		"result":        req.ResultJson,
	})

	// 更新 Operation 状态
	models.DB.Model(&models.Operation{}).Where("id = ?", task.OperationID).Updates(map[string]interface{}{
		"status":        req.Status,
		"error_message": req.ErrorMessage,
	})

	// 解析 payload 获取目标 ID
	var payload map[string]string
	json.Unmarshal([]byte(task.Payload), &payload)

	// 联动状态更新
	switch task.Type {
	case "start_group":
		if gid, ok := payload["group_id"]; ok {
			if req.Status == "success" {
				models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
					"current_state": "running", "last_error": "", "last_observed_at": time.Now(),
				})
			} else {
				models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
					"current_state": "error", "last_error": req.ErrorMessage, "last_observed_at": time.Now(),
				})
			}
		}
	case "stop_group":
		if gid, ok := payload["group_id"]; ok {
			models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
				"current_state": "stopped", "last_observed_at": time.Now(),
			})
		}
	case "restart_group":
		if gid, ok := payload["group_id"]; ok {
			if req.Status == "success" {
				models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
					"current_state": "running", "last_error": "", "last_observed_at": time.Now(),
				})
			} else {
				models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
					"current_state": "error", "last_error": req.ErrorMessage,
				})
			}
		}
	case "replace_proxy":
		if gid, ok := payload["group_id"]; ok {
			if req.Status == "success" {
				models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
					"current_state": "running", "last_error": "",
				})
			} else {
				models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", gid).Updates(map[string]interface{}{
					"current_state": "error", "last_error": req.ErrorMessage,
				})
			}
		}
	case "start_browser":
		if bid, ok := payload["browser_id"]; ok {
			if req.Status == "success" {
				// 从 result_json 解析端口
				var result map[string]interface{}
				json.Unmarshal([]byte(req.ResultJson), &result)
				updates := map[string]interface{}{"status": "running", "last_error": ""}
				if vp, ok := result["vnc_port"]; ok {
					if vpf, ok := vp.(float64); ok {
						updates["vnc_port"] = int(vpf)
					}
				}
				if cp, ok := result["cdp_port"]; ok {
					if cpf, ok := cp.(float64); ok {
						updates["cdp_port"] = int(cpf)
					}
				}
				models.DB.Model(&models.BrowserInstance{}).Where("id = ?", bid).Updates(updates)
			} else {
				models.DB.Model(&models.BrowserInstance{}).Where("id = ?", bid).Updates(map[string]interface{}{
					"status": "error", "last_error": req.ErrorMessage,
				})
			}
		}
	case "stop_browser":
		if bid, ok := payload["browser_id"]; ok {
			models.DB.Model(&models.BrowserInstance{}).Where("id = ?", bid).Update("status", "stopped")
		}
	}

	return &pb.TaskResultResponse{Success: true}, nil
}
