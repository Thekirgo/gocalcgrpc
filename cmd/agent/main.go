package main

import (
	"gocalc/internal/grpc"
	pb "gocalc/proto"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"gocalc/internal/database"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

var (
	TIME_ADDITION_MS        int
	TIME_SUBTRACTION_MS     int
	TIME_MULTIPLICATIONS_MS int
	TIME_DIVISIONS_MS       int
	COMPUTING_POWER         int
)

func loadConfig() {
	envFiles := []string{".env", "../.env", "../../.env"}
	for _, file := range envFiles {
		if err := godotenv.Load(file); err == nil {
			break
		}
	}

	var err error
	TIME_ADDITION_MS, err = strconv.Atoi(getEnvOrDefault("TIME_ADDITION_MS", "510"))
	if err != nil {
		log.Fatal("Invalid TIME_ADDITION_MS")
	}

	TIME_SUBTRACTION_MS, err = strconv.Atoi(getEnvOrDefault("TIME_SUBTRACTION_MS", "520"))
	if err != nil {
		log.Fatal("Invalid TIME_SUBTRACTION_MS")
	}

	TIME_MULTIPLICATIONS_MS, err = strconv.Atoi(getEnvOrDefault("TIME_MULTIPLICATIONS_MS", "530"))
	if err != nil {
		log.Fatal("Invalid TIME_MULTIPLICATIONS_MS")
	}

	TIME_DIVISIONS_MS, err = strconv.Atoi(getEnvOrDefault("TIME_DIVISIONS_MS", "540"))
	if err != nil {
		log.Fatal("Invalid TIME_DIVISIONS_MS")
	}

	COMPUTING_POWER, err = strconv.Atoi(getEnvOrDefault("COMPUTING_POWER", "4"))
	if err != nil {
		log.Fatal("Invalid COMPUTING_POWER")
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	log.Printf("ВНИМАНИЕ: Переменная окружения %s не установлена, используется значение по умолчанию", key)
	return defaultValue
}

func main() {
	loadConfig()

	database.GetDB()

	orchestratorAddr := getEnvOrDefault("ORCHESTRATOR_GRPC_ADDR", "localhost:8081")

	client, err := grpc.NewCalculatorClient(orchestratorAddr)
	if err != nil {
		log.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	sem := make(chan struct{}, COMPUTING_POWER)
	var wg sync.WaitGroup

	log.Printf("Агент запущен с COMPUTING_POWER: %d", COMPUTING_POWER)
	log.Printf("Агент подключается к gRPC серверу по адресу %s", orchestratorAddr)

	for i := 0; i < COMPUTING_POWER; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			agentID := uuid.New().String()

			for {
				sem <- struct{}{}
				processTaskGRPC(client, workerID, agentID)
				<-sem
			}
		}(i)
	}

	wg.Wait()
}

// обрабатывает задачу через gRPC
func processTaskGRPC(client *grpc.CalculatorClient, workerID int, agentID string) {
	var task *pb.Task
	var err error

	maxRetries := 3
	retryDelay := 1 * time.Second

	for retry := 0; retry < maxRetries; retry++ {
		task, err = client.GetTask(agentID)

		if err == nil {
			if task == nil {
				time.Sleep(time.Second)
				return
			}

			break
		}

		log.Printf("Worker %d (агент %s): Ошибка получения задачи (попытка %d/%d): %v",
			workerID, agentID, retry+1, maxRetries, err)

		if retry == maxRetries-1 {
			log.Printf("Worker %d (агент %s): Достигнуто максимальное количество попыток получения задачи",
				workerID, agentID)
			time.Sleep(time.Second)
			return
		}

		time.Sleep(retryDelay)
		retryDelay *= 2
	}

	if task == nil {
		log.Printf("Worker %d (агент %s): Получен nil для задачи после проверки на ошибки", workerID, agentID)
		time.Sleep(time.Second)
		return
	}

	log.Printf("Worker %d (агент %s): Получена задача: ID=%s, операция=%s, время=%d мс, arg1=%f, arg2=%f",
		workerID, agentID, task.Id, task.Operation, task.OperationTime, task.Arg1, task.Arg2)

	result := calculateResultWithTime(task.Operation, task.Arg1, task.Arg2, int(task.OperationTime))

	log.Printf("Worker %d (агент %s): Завершено вычисление для задачи %s, результат: %f",
		workerID, agentID, task.Id, result)

	for retry := 0; retry < maxRetries; retry++ {
		log.Printf("Worker %d (агент %s): Отправка результата для задачи %s, попытка %d/%d",
			workerID, agentID, task.Id, retry+1, maxRetries)

		err = client.SubmitTaskResult(task.Id, result)

		if err == nil {
			log.Printf("Worker %d (агент %s): Результат для задачи %s успешно отправлен",
				workerID, agentID, task.Id)
			break
		}

		log.Printf("Worker %d (агент %s): Ошибка отправки результата (попытка %d/%d): %v",
			workerID, agentID, retry+1, maxRetries, err)

		if retry == maxRetries-1 {
			log.Printf("Worker %d (агент %s): Не удалось отправить результат после %d попыток",
				workerID, agentID, maxRetries)
			break
		}

		time.Sleep(retryDelay)
		retryDelay *= 2
	}
}

// calculateResult вычисляет результат операции с симуляцией задержки
func calculateResult(operation string, arg1, arg2 float64) float64 {
	var delay time.Duration
	switch operation {
	case "+":
		delay = time.Duration(TIME_ADDITION_MS) * time.Millisecond
		log.Printf("Операция сложения: %f + %f с задержкой %v мс", arg1, arg2, TIME_ADDITION_MS)
	case "-":
		delay = time.Duration(TIME_SUBTRACTION_MS) * time.Millisecond
		log.Printf("Операция вычитания: %f - %f с задержкой %v мс", arg1, arg2, TIME_SUBTRACTION_MS)
	case "*":
		delay = time.Duration(TIME_MULTIPLICATIONS_MS) * time.Millisecond
		log.Printf("Операция умножения: %f * %f с задержкой %v мс", arg1, arg2, TIME_MULTIPLICATIONS_MS)
	case "/":
		delay = time.Duration(TIME_DIVISIONS_MS) * time.Millisecond
		log.Printf("Операция деления: %f / %f с задержкой %v мс", arg1, arg2, TIME_DIVISIONS_MS)
	}

	startTime := time.Now()
	time.Sleep(delay)
	elapsedTime := time.Since(startTime)
	log.Printf("Операция %s завершена за %v", operation, elapsedTime)

	switch operation {
	case "+":
		return arg1 + arg2
	case "-":
		return arg1 - arg2
	case "*":
		return arg1 * arg2
	case "/":
		if arg2 == 0 {
			return 0
		}
		return arg1 / arg2
	default:
		return 0
	}
}

// calculateResultWithTime вычисляет результат с указанной задержкой в миллисекундах
func calculateResultWithTime(operation string, arg1, arg2 float64, operationTimeMs int) float64 {
	delay := time.Duration(operationTimeMs) * time.Millisecond

	log.Printf("НАЧАЛО выполнения операции %s: %f %s %f с задержкой %d мс",
		operation, arg1, operation, arg2, operationTimeMs)

	startTime := time.Now()
	time.Sleep(delay)
	elapsedTime := time.Since(startTime)
	log.Printf("ЗАВЕРШЕНИЕ операции %s: результат = %f, выполнялось %v",
		operation, getOperationResult(operation, arg1, arg2), elapsedTime)

	return getOperationResult(operation, arg1, arg2)
}

func getOperationResult(operation string, arg1, arg2 float64) float64 {
	switch operation {
	case "+":
		return arg1 + arg2
	case "-":
		return arg1 - arg2
	case "*":
		return arg1 * arg2
	case "/":
		if arg2 == 0 {
			return 0
		}
		return arg1 / arg2
	default:
		return 0
	}
}
