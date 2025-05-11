package integration_tests

import (
	"context"
	internalgrpc "gocalc/internal/grpc"
	"gocalc/internal/models"
	"gocalc/internal/orchestrator"
	pb "gocalc/proto"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

var _ = func() bool {
	orchestrator.SaveExpressionFunc = func(expression *models.Expression, userID int) error {
		return nil
	}
	return true
}()

func setupGRPCServer(t *testing.T) (*orchestrator.TaskManager, *bufconn.Listener, func()) {
	lis := bufconn.Listen(bufSize)
	taskManager := orchestrator.NewTaskManager()

	srv := grpc.NewServer()
	pb.RegisterCalculatorServer(srv, internalgrpc.NewCalculatorServer(taskManager))

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Errorf("Ошибка запуска сервера: %v", err)
		}
	}()

	return taskManager, lis, func() {
		srv.Stop()
		lis.Close()
	}
}

// bufDialer реализует функцию для установки соединения через bufconn
func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, s string) (net.Conn, error) {
		return lis.Dial()
	}
}

// TestGRPCGetTask проверяет получение задачи через gRPC
func TestGRPCGetTask(t *testing.T) {
	taskManager, lis, cleanup := setupGRPCServer(t)
	defer cleanup()

	// Создаем тестовое выражение
	_, err := taskManager.CreateExpression("2+3*4", 1)
	if err != nil {
		t.Fatalf("Ошибка создания выражения: %v", err)
	}

	// Устанавливаем соединение
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Ошибка подключения к серверу: %v", err)
	}
	defer conn.Close()

	client := pb.NewCalculatorClient(conn)

	task, err := client.GetTask(ctx, &pb.TaskRequest{AgentId: "test-agent"})
	if err != nil {
		t.Fatalf("Ошибка при получении задачи: %v", err)
	}

	if task == nil {
		t.Fatalf("Получена пустая задача")
	}

	if task.Operation != "*" {
		t.Errorf("Ожидалась операция умножения (*), получена: %s", task.Operation)
	}

	if task.Priority != 2 {
		t.Errorf("Ожидался приоритет 2, получен: %d", task.Priority)
	}
}

// TestGRPCSubmitTaskResult проверяет отправку результата через gRPC
func TestGRPCSubmitTaskResult(t *testing.T) {
	taskManager, lis, cleanup := setupGRPCServer(t)
	defer cleanup()

	// Создаем тестовое выражение
	exprID, err := taskManager.CreateExpression("5+5", 1)
	if err != nil {
		t.Fatalf("Ошибка создания выражения: %v", err)
	}

	task, found := taskManager.GetNextTask()
	if !found {
		t.Fatalf("Не удалось получить задачу")
	}

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Ошибка подключения к серверу: %v", err)
	}
	defer conn.Close()

	client := pb.NewCalculatorClient(conn)

	response, err := client.SubmitTaskResult(ctx, &pb.TaskResult{
		Id:     task.ID,
		Result: 10.0, // 5+5=10
	})

	if err != nil {
		t.Fatalf("Ошибка отправки результата: %v", err)
	}

	if !response.Success {
		t.Errorf("Ожидался успешный ответ, получена ошибка: %s", response.ErrorMessage)
	}

	// Проверяем, что выражение обработано
	expr, exists := taskManager.GetExpression(exprID)
	if !exists {
		t.Fatalf("Выражение не найдено после отправки результата")
	}

	if expr.Status != "COMPLETED" {
		t.Errorf("Ожидался статус COMPLETED, получен: %s", expr.Status)
	}

	if expr.Result != 10.0 {
		t.Errorf("Ожидался результат 10.0, получен: %f", expr.Result)
	}
}

// TestGRPCNoTasks проверяет поведение при отсутствии задач
func TestGRPCNoTasks(t *testing.T) {
	_, lis, cleanup := setupGRPCServer(t)
	defer cleanup()

	// Устанавливаем соединение
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Ошибка подключения к серверу: %v", err)
	}
	defer conn.Close()

	// Создаем клиента
	client := pb.NewCalculatorClient(conn)

	// Пытаемся получить задачу
	_, err = client.GetTask(ctx, &pb.TaskRequest{AgentId: "test-agent"})

	// Ожидаем ошибку NotFound
	if err == nil {
		t.Fatalf("Ожидалась ошибка NotFound, получен успешный ответ")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Ожидалась ошибка статуса, получена: %v", err)
	}

	if st.Code() != codes.NotFound {
		t.Errorf("Ожидался код NotFound, получен: %s", st.Code())
	}
}

// TestGRPCInvalidTaskResult проверяет поведение при отправке результата для несуществующей задачи
func TestGRPCInvalidTaskResult(t *testing.T) {
	_, lis, cleanup := setupGRPCServer(t)
	defer cleanup()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Ошибка подключения к серверу: %v", err)
	}
	defer conn.Close()

	client := pb.NewCalculatorClient(conn)

	response, err := client.SubmitTaskResult(ctx, &pb.TaskResult{
		Id:     "non-existent-task",
		Result: 42.0,
	})

	if err != nil {
		t.Fatalf("Ожидался ответ с ошибкой внутри, получена ошибка gRPC: %v", err)
	}

	// Проверяем, что получен ответ с флагом неуспешности
	if response.Success {
		t.Errorf("Ожидался неуспешный ответ для несуществующей задачи")
	}
}

// TestGRPCZeroResult проверяет корректную обработку нулевого результата через gRPC
func TestGRPCZeroResult(t *testing.T) {
	taskManager, lis, cleanup := setupGRPCServer(t)
	defer cleanup()

	// Создаем тестовое выражение 2-2=0
	exprID, err := taskManager.CreateExpression("2-2", 1)
	if err != nil {
		t.Fatalf("Ошибка создания выражения: %v", err)
	}

	// Устанавливаем соединение
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Ошибка подключения к серверу: %v", err)
	}
	defer conn.Close()

	// Создаем клиента
	client := pb.NewCalculatorClient(conn)

	// Получаем задачу
	task, err := client.GetTask(ctx, &pb.TaskRequest{AgentId: "test-agent"})
	if err != nil {
		t.Fatalf("Ошибка при получении задачи: %v", err)
	}

	// Проверяем, что задача не пустая
	if task == nil {
		t.Fatalf("Получена пустая задача")
	}

	// Отправляем результат 0.0
	response, err := client.SubmitTaskResult(ctx, &pb.TaskResult{
		Id:     task.Id,
		Result: 0.0,
	})

	if err != nil {
		t.Fatalf("Ошибка отправки результата: %v", err)
	}

	// Проверяем успешность обработки
	if !response.Success {
		t.Errorf("Ожидался успешный ответ, получена ошибка: %s", response.ErrorMessage)
	}

	// Проверяем, что выражение обработано
	expr, exists := taskManager.GetExpression(exprID)
	if !exists {
		t.Fatalf("Выражение не найдено после отправки результата")
	}

	if expr.Status != "COMPLETED" {
		t.Errorf("Ожидался статус COMPLETED, получен: %s", expr.Status)
	}

	if expr.Result != 0.0 {
		t.Errorf("Ожидался результат 0.0, получен: %f", expr.Result)
	}
}

// TestGRPC_ComplexExpressionWithDependencies проверяет выражение с зависимостями (2+2+(3/3))
func TestGRPC_ComplexExpressionWithDependencies(t *testing.T) {
	taskManager, lis, cleanup := setupGRPCServer(t)
	defer cleanup()

	// Создаем выражение (2+2+(3/3))
	exprID, err := taskManager.CreateExpression("2+2+(3/3)", 1)
	if err != nil {
		t.Fatalf("Ошибка создания выражения: %v", err)
	}

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Ошибка подключения к серверу: %v", err)
	}
	defer conn.Close()

	client := pb.NewCalculatorClient(conn)

	// Обрабатываем все задачи по очереди
	for {
		task, err := client.GetTask(ctx, &pb.TaskRequest{AgentId: "test-agent"})
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.NotFound {
				break // Все задачи обработаны
			}
			t.Fatalf("Ошибка получения задачи: %v", err)
		}

		var result float64
		switch task.Operation {
		case "+":
			result = task.Arg1 + task.Arg2
		case "-":
			result = task.Arg1 - task.Arg2
		case "*":
			result = task.Arg1 * task.Arg2
		case "/":
			if task.Arg2 == 0 {
				result = 0
			} else {
				result = task.Arg1 / task.Arg2
			}
		}
		_, err = client.SubmitTaskResult(ctx, &pb.TaskResult{
			Id:     task.Id,
			Result: result,
		})
		if err != nil {
			t.Fatalf("Ошибка отправки результата: %v", err)
		}
	}

	expr, exists := taskManager.GetExpression(exprID)
	if !exists {
		t.Fatalf("Выражение не найдено после обработки")
	}
	if expr.Status != "COMPLETED" {
		t.Errorf("Ожидался статус COMPLETED, получен: %s", expr.Status)
	}
	if expr.Result != 5.0 {
		t.Errorf("Ожидался результат 5.0, получен: %f", expr.Result)
	}
}
