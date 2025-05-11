package grpc

import (
	"context"
	pb "gocalc/proto"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// CalculatorClient представляет gRPC клиент для агента
type CalculatorClient struct {
	client pb.CalculatorClient
	conn   *grpc.ClientConn
}

// NewCalculatorClient создает новый экземпляр gRPC клиента
func NewCalculatorClient(serverAddr string) (*CalculatorClient, error) {
	// Добавляем опции для больших сообщений и увеличиваем таймаут
	conn, err := grpc.Dial(
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(16*1024*1024), // 16MB
			grpc.MaxCallSendMsgSize(16*1024*1024), // 16MB
		),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}

	return &CalculatorClient{
		client: pb.NewCalculatorClient(conn),
		conn:   conn,
	}, nil
}

// Close закрывает соединение с сервером
func (c *CalculatorClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// GetTask запрашивает задачу у оркестратора
func (c *CalculatorClient) GetTask(agentID string) (*pb.Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10) // Увеличиваем таймаут
	defer cancel()

	task, err := c.client.GetTask(ctx, &pb.TaskRequest{
		AgentId: agentID,
	})

	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.NotFound {
			return nil, nil // Нет доступных задач
		}
		log.Printf("Агент %s: ошибка получения задачи: %v", agentID, err)
		return nil, err
	}

	log.Printf("Агент %s: получена задача %s для выполнения", agentID, task.Id)
	return task, nil
}

// SubmitTaskResult отправляет результат вычисления оркестратору
func (c *CalculatorClient) SubmitTaskResult(taskID string, result float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10) // Увеличиваем таймаут
	defer cancel()

	log.Printf("Отправка результата для задачи %s: %f", taskID, result)
	res, err := c.client.SubmitTaskResult(ctx, &pb.TaskResult{
		Id:     taskID,
		Result: result,
	})

	if err != nil {
		log.Printf("Ошибка отправки результата для задачи %s: %v", taskID, err)
		return err
	}

	if !res.Success {
		log.Printf("Ошибка обработки результата задачи %s: %s", taskID, res.ErrorMessage)
		return status.Error(codes.Internal, res.ErrorMessage)
	}

	log.Printf("Результат задачи %s успешно обработан", taskID)
	return nil
}
