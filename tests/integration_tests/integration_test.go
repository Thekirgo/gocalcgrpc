package integration_tests

import (
	"context"
	"fmt"
	"gocalc/internal/grpc"
	"gocalc/internal/models"
	"gocalc/internal/orchestrator"
	pb "gocalc/proto"
	"net"
	"sync"
	"testing"
	"time"

	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var _ = func() bool {
	orchestrator.SaveExpressionFunc = func(expression *models.Expression, userID int) error {
		return nil // мок
	}
	return true
}()

func setupIntegrationTest(t *testing.T) (*orchestrator.TaskManager, pb.CalculatorClient, func()) {
	lis := bufconn.Listen(1024 * 1024)

	taskManager := orchestrator.NewTaskManager()
	server := ggrpc.NewServer()
	pb.RegisterCalculatorServer(server, grpc.NewCalculatorServer(taskManager))

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	conn, err := ggrpc.DialContext(
		context.Background(),
		"",
		ggrpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		ggrpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Ошибка создания клиента: %v", err)
	}

	client := pb.NewCalculatorClient(conn)

	cleanup := func() {
		conn.Close()
		server.Stop()
		lis.Close()
	}

	return taskManager, client, cleanup
}

// runGRPCAgent симулирует работу агента через gRPC
func runGRPCAgent(t *testing.T, client pb.CalculatorClient, wg *sync.WaitGroup, id string) {
	defer wg.Done()

	ctx := context.Background()
	for {
		task, err := client.GetTask(ctx, &pb.TaskRequest{AgentId: id})
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.NotFound {
				break // Все задачи обработаны
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		// Выполняем вычисление
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
				result = 0 // Избегаем деления на ноль
			} else {
				result = task.Arg1 / task.Arg2
			}
		}
		// Симулируем время выполнения
		time.Sleep(time.Duration(task.OperationTime) * time.Millisecond)
		// Отправляем результат
		_, err = client.SubmitTaskResult(ctx, &pb.TaskResult{
			Id:     task.Id,
			Result: result,
		})
		if err != nil {
			t.Logf("Агент %s: ошибка отправки результата: %v", id, err)
		}
	}
}

// TestFullExpressionCalculation проверяет полный цикл вычисления выражения
func TestFullExpressionCalculation(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		expected   float64
	}{
		{
			name:       "simple addition",
			expression: "2+3",
			expected:   5.0,
		},
		{
			name:       "complex expression",
			expression: "2+3*4",
			expected:   14.0,
		},
		{
			name:       "parentheses",
			expression: "(2+3)*4",
			expected:   20.0,
		},
		{
			name:       "multiple operations",
			expression: "1+2*3-4/2",
			expected:   5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskManager, client, cleanup := setupIntegrationTest(t)
			defer cleanup()

			// Создаем выражение
			exprID, err := taskManager.CreateExpression(tt.expression, 1)
			if err != nil {
				t.Fatalf("Ошибка создания выражения: %v", err)
			}

			// Запускаем агента для обработки задач
			var wg sync.WaitGroup
			wg.Add(1)
			go runGRPCAgent(t, client, &wg, "test-agent")

			// Ждем завершения работы агента или таймаут
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Агент завершил работу
			case <-time.After(10 * time.Second):
				t.Fatal("Таймаут ожидания результата")
			}

			// Проверяем результат
			expr, exists := taskManager.GetExpression(exprID)
			if !exists {
				t.Fatalf("Выражение не найдено после обработки")
			}

			if expr.Status != "COMPLETED" {
				t.Errorf("Неверный статус выражения: %s", expr.Status)
			}

			if expr.Result != tt.expected {
				t.Errorf("Неверный результат: ожидалось %f, получено %f", tt.expected, expr.Result)
			}
		})
	}

	// Добавить тест на выражение (2+2+(3/3))
	t.Run("complex dependencies (2+2+(3/3))", func(t *testing.T) {
		taskManager, client, cleanup := setupIntegrationTest(t)
		defer cleanup()

		exprID, err := taskManager.CreateExpression("2+2+(3/3)", 1)
		if err != nil {
			t.Fatalf("Ошибка создания выражения: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go runGRPCAgent(t, client, &wg, "test-agent")

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Таймаут ожидания результата")
		}

		expr, exists := taskManager.GetExpression(exprID)
		if !exists {
			t.Fatalf("Выражение не найдено после обработки")
		}
		if expr.Status != "COMPLETED" {
			t.Errorf("Неверный статус выражения: %s", expr.Status)
		}
		if expr.Result != 5.0 {
			t.Errorf("Неверный результат: ожидалось 5.0, получено %f", expr.Result)
		}
	})
}

// TestConcurrentExpressionProcessing проверяет параллельное вычисление нескольких выражений
func TestConcurrentExpressionProcessing(t *testing.T) {
	taskManager, client, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Создаем несколько выражений
	expressions := []struct {
		expr     string
		expected float64
	}{
		{"2+3", 5.0},
		{"4*5", 20.0},
		{"10-2", 8.0},
		{"8/2", 4.0},
		{"2+2*2", 6.0},
	}

	var exprIDs []string
	for _, e := range expressions {
		id, err := taskManager.CreateExpression(e.expr, 1)
		if err != nil {
			t.Fatalf("Ошибка создания выражения %s: %v", e.expr, err)
		}
		exprIDs = append(exprIDs, id)
	}

	// Запускаем несколько агентов для обработки задач
	var wg sync.WaitGroup
	numAgents := 3
	wg.Add(numAgents)
	for i := 0; i < numAgents; i++ {
		go runGRPCAgent(t, client, &wg, fmt.Sprintf("test-agent-%d", i))
	}

	// Ждем завершения работы агентов или таймаут
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Таймаут ожидания результатов")
	}

	for i, id := range exprIDs {
		expr, exists := taskManager.GetExpression(id)
		if !exists {
			t.Fatalf("Выражение %s не найдено после обработки", id)
		}
		if expr.Status != "COMPLETED" {
			t.Errorf("Неверный статус выражения %s: %s", id, expr.Status)
		}
		if expr.Result != expressions[i].expected {
			t.Errorf("Неверный результат для %s: ожидалось %f, получено %f",
				expressions[i].expr, expressions[i].expected, expr.Result)
		}
	}
}
