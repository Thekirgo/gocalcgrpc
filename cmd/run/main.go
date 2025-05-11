package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func isPortFree(port string) bool {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

func waitForOrchestratorGRPC(port string, timeout time.Duration) bool {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return false
		}

		conn, err := net.DialTimeout("tcp", "localhost:"+port, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			time.Sleep(1 * time.Second)
			return true
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// чтение переменной окружения
func getEnvOrDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

func main() {
	envFiles := []string{".env", "../.env", "../../.env"}

	for _, file := range envFiles {
		if _, err := os.Stat(file); err == nil {
			content, err := os.ReadFile(file)
			if err == nil {
				log.Printf("Загружен файл .env: %s (размер: %d байт)", file, len(content))

				if len(content) >= 2 && content[0] == 0xFF && content[1] == 0xFE {
					log.Printf("Обнаружена кодировка UTF-16 LE, конвертируем в UTF-8")
					utf8Content := make([]byte, 0, len(content)/2)

					for i := 2; i < len(content); i += 2 {
						if i+1 < len(content) {
							charCode := uint16(content[i]) | (uint16(content[i+1]) << 8)

							if charCode == 0 {
								continue
							}

							if charCode < 128 {
								utf8Content = append(utf8Content, byte(charCode))
							} else if charCode < 2048 {
								utf8Content = append(utf8Content, byte(192|(charCode>>6)))
								utf8Content = append(utf8Content, byte(128|(charCode&63)))
							} else {
								utf8Content = append(utf8Content, byte(224|(charCode>>12)))
								utf8Content = append(utf8Content, byte(128|((charCode>>6)&63)))
								utf8Content = append(utf8Content, byte(128|(charCode&63)))
							}
						}
					}

					content = utf8Content
					log.Printf("Содержимое после конвертации в UTF-8: %s", string(content))
				} else if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
					log.Printf("Обнаружен BOM (Byte Order Mark) в начале файла, удаляем его")
					content = content[3:]
				}

				lines := strings.Split(string(content), "\n")
				for _, line := range lines {
					if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
						parts := strings.SplitN(line, "=", 2)
						if len(parts) == 2 {
							key := strings.TrimSpace(parts[0])
							value := strings.TrimSpace(parts[1])
							os.Setenv(key, value)
							log.Printf("Установлена переменная окружения: %s=%s", key, value)
						}
					}
				}
				break
			}
		}
	}

	timingVars := []string{
		"TIME_ADDITION_MS",
		"TIME_SUBTRACTION_MS",
		"TIME_MULTIPLICATIONS_MS",
		"TIME_DIVISIONS_MS",
	}

	for _, varName := range timingVars {
		value := os.Getenv(varName)

		if value == "" {
			log.Fatalf("Переменная окружения %s не установлена! Убедитесь, что файл .env существует и содержит корректные значения.", varName)
		}
	}

	goCmd := "go"

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(3)

	done := make(chan struct{})

	if !isPortFree(getEnvOrDefault("ORCHESTRATOR_HTTP_PORT", "8080")) {
		log.Fatalf("Порт %s уже занят. Завершаем работу.", getEnvOrDefault("ORCHESTRATOR_HTTP_PORT", "8080"))
	}
	if !isPortFree(getEnvOrDefault("ORCHESTRATOR_GRPC_PORT", "8081")) {
		log.Fatalf("Порт %s уже занят. Завершаем работу.", getEnvOrDefault("ORCHESTRATOR_GRPC_PORT", "8081"))
	}
	if !isPortFree(getEnvOrDefault("AUTH_SERVICE_PORT", "8083")) {
		log.Fatalf("Порт %s уже занят. Завершаем работу.", getEnvOrDefault("AUTH_SERVICE_PORT", "8083"))
	}

	log.Println("Запуск оркестратора...")
	orchestratorCmd := exec.Command(goCmd, "run", "./cmd/orchestrator")

	orchestratorEnv := os.Environ()
	orchestratorEnv = append(orchestratorEnv, "ORCHESTRATOR_HTTP_PORT="+getEnvOrDefault("ORCHESTRATOR_HTTP_PORT", "8080"))
	orchestratorEnv = append(orchestratorEnv, "ORCHESTRATOR_GRPC_PORT="+getEnvOrDefault("ORCHESTRATOR_GRPC_PORT", "8081"))

	for _, envVar := range []string{
		"TIME_ADDITION_MS",
		"TIME_SUBTRACTION_MS",
		"TIME_MULTIPLICATIONS_MS",
		"TIME_DIVISIONS_MS",
	} {
		value := os.Getenv(envVar)
		if value != "" {
			orchestratorEnv = append(orchestratorEnv, envVar+"="+value)
		} else {
			log.Fatalf("Переменная окружения %s не установлена для оркестратора! Убедитесь, что файл .env существует и содержит корректные значения.", envVar)
		}
	}

	orchestratorCmd.Env = orchestratorEnv

	orchestratorCmd.Stdout = os.Stdout
	orchestratorCmd.Stderr = os.Stderr

	err := orchestratorCmd.Start()
	if err != nil {
		log.Fatalf("Ошибка запуска оркестратора: %v", err)
	}

	log.Println("Ожидание готовности оркестратора...")
	if !waitForOrchestratorGRPC(getEnvOrDefault("ORCHESTRATOR_GRPC_PORT", "8081"), 20*time.Second) {
		orchestratorCmd.Process.Kill()
		log.Fatalf("Превышено время ожидания запуска оркестратора")
	}

	log.Println("Оркестратор готов. Запуск агента...")
	log.Println("gRPC агент будет работать на порту 8081")
	agentCmd := exec.Command(goCmd, "run", "./cmd/agent")

	agentEnv := os.Environ()
	agentEnv = append(agentEnv, "ORCHESTRATOR_GRPC_ADDR=localhost:"+getEnvOrDefault("ORCHESTRATOR_GRPC_PORT", "8081"))

	for _, envVar := range []string{
		"TIME_ADDITION_MS",
		"TIME_SUBTRACTION_MS",
		"TIME_MULTIPLICATIONS_MS",
		"TIME_DIVISIONS_MS",
		"COMPUTING_POWER",
	} {
		value := os.Getenv(envVar)
		if value != "" {
			agentEnv = append(agentEnv, envVar+"="+value)
		} else if envVar == "COMPUTING_POWER" {
			agentEnv = append(agentEnv, envVar+"=4")
			log.Printf("Для агента используется значение COMPUTING_POWER по умолчанию: 4")
		} else {
			log.Fatalf("Переменная окружения %s не установлена для агента! Убедитесь, что файл .env существует и содержит корректные значения.", envVar)
		}
	}

	agentCmd.Env = agentEnv
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr

	err = agentCmd.Start()
	if err != nil {
		orchestratorCmd.Process.Kill()
		log.Fatalf("Ошибка запуска агента: %v", err)
	}

	log.Println("Запуск сервиса авторизации...")
	authPort := getEnvOrDefault("AUTH_SERVICE_PORT", "8083")

	var authServiceCmd *exec.Cmd

	nullFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		authServiceCmd = exec.Command(goCmd, "run", "./cmd/calc_service")
		authServiceCmd.Stdout = os.Stdout
		authServiceCmd.Stderr = os.Stderr
		authServiceCmd.Env = append(os.Environ(), "CALC_SERVICE_PORT="+authPort)
	} else {
		defer nullFile.Close()

		authServiceCmd = exec.Command(goCmd, "run", "./cmd/calc_service")
		authServiceCmd.Stdout = nullFile
		authServiceCmd.Stderr = nullFile
		authServiceCmd.Env = append(os.Environ(), "CALC_SERVICE_PORT="+authPort)
	}

	// Запускаем сервис авторизации
	err = authServiceCmd.Start()
	if err != nil {
		orchestratorCmd.Process.Kill()
		agentCmd.Process.Kill()
		log.Fatalf("Ошибка запуска сервиса авторизации: %v", err)
	}

	log.Printf("Сервис авторизации запущен на порту %s\n", authPort)
	time.Sleep(1 * time.Second)
	log.Println("Все сервисы успешно запущены и готовы к работе")

	go func() {
		<-sigs
		log.Println("Получен сигнал завершения. Завершаем процессы...")

		if agentCmd.Process != nil {
			agentCmd.Process.Kill()
		}

		if orchestratorCmd.Process != nil {
			orchestratorCmd.Process.Kill()
		}

		if authServiceCmd.Process != nil {
			authServiceCmd.Process.Kill()
		}

		close(done)
	}()

	go func() {
		defer wg.Done()
		err = orchestratorCmd.Wait()
		if err != nil {
			fmt.Printf("Оркестратор завершился с ошибкой: %v\n", err)
		} else {
			fmt.Println("Оркестратор успешно завершил работу")
		}
	}()

	go func() {
		defer wg.Done()
		err = agentCmd.Wait()
		if err != nil {
			fmt.Printf("Агент завершился с ошибкой: %v\n", err)
		} else {
			fmt.Println("Агент успешно завершил работу")
		}
	}()

	go func() {
		defer wg.Done()
		err = authServiceCmd.Wait()
		if err != nil {
			fmt.Printf("Сервис авторизации завершился с ошибкой: %v\n", err)
		} else {
			fmt.Println("Сервис авторизации успешно завершил работу")
		}
	}()

	go func() {
		wg.Wait()
		close(done)
	}()

	<-done
	fmt.Println("Все процессы завершены.")
}
