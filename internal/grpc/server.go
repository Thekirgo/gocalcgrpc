package grpc

import (
	"context"
	"gocalc/internal/orchestrator"
	pb "gocalc/proto"
	"log"
	"net"
	"sync"

	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// CalculatorServer реализует gRPC сервер для вычисления
type CalculatorServer struct {
	pb.UnimplementedCalculatorServer
	taskManager *orchestrator.TaskManager
	mu          sync.Mutex
}

// NewCalculatorServer создает новый экземпляр gRPC сервера
func NewCalculatorServer(taskManager *orchestrator.TaskManager) *CalculatorServer {
	return &CalculatorServer{
		taskManager: taskManager,
	}
}

// GetTask возвращает задачу для вычисления агенту
func (s *CalculatorServer) GetTask(ctx context.Context, req *pb.TaskRequest) (*pb.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, found := s.taskManager.GetNextTask()
	if !found {
		return nil, status.Error(codes.NotFound, "Нет доступных задач")
	}

	log.Printf("GetTask gRPC: Отправка задачи агенту %s: ID=%s, операция=%s, время=%d мс, arg1=%f, arg2=%f",
		req.AgentId, task.ID, task.Operation, task.OperationTime, task.Arg1, task.Arg2)

	return &pb.Task{
		Id:            task.ID,
		Arg1:          task.Arg1,
		Arg2:          task.Arg2,
		Operation:     task.Operation,
		OperationTime: int32(task.OperationTime),
		Priority:      int32(task.Priority),
	}, nil
}

// SubmitTaskResult принимает результат вычисления от агента
func (s *CalculatorServer) SubmitTaskResult(ctx context.Context, result *pb.TaskResult) (*pb.TaskResultResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("Получен результат задачи %s от агента: %f", result.Id, result.Result)

	err := s.taskManager.SubmitTaskResult(orchestrator.TaskResult{
		ID:     result.Id,
		Result: result.Result,
	})

	if err != nil {
		log.Printf("Ошибка при обработке результата задачи %s: %v", result.Id, err)
		return &pb.TaskResultResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	log.Printf("Результат задачи %s успешно обработан", result.Id)
	return &pb.TaskResultResponse{
		Success: true,
	}, nil
}

// StartServer запускает gRPC сервер
func StartServer(address string, taskManager *orchestrator.TaskManager) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	// Настройки для keepalive и размеров сообщений
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(16 * 1024 * 1024), // 16MB
		grpc.MaxSendMsgSize(16 * 1024 * 1024), // 16MB
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     time.Minute,
			MaxConnectionAge:      5 * time.Minute,
			MaxConnectionAgeGrace: 20 * time.Second,
			Time:                  20 * time.Second,
			Timeout:               10 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	s := grpc.NewServer(opts...)
	pb.RegisterCalculatorServer(s, NewCalculatorServer(taskManager))

	log.Printf("gRPC сервер запущен на %s", address)
	return s.Serve(lis)
}
