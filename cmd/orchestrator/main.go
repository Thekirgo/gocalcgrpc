package main

import (
	"gocalc/internal/database"
	"gocalc/internal/grpc"
	"gocalc/internal/orchestrator"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

func main() {
	database.GetDB()

	envFiles := []string{".env", "../.env", "../../.env"}
	for _, file := range envFiles {
		if err := godotenv.Load(file); err == nil {
			//log.Printf("Загружен файл с переменными окружения: %s", file)
			break
		}
	}

	authServicePort := os.Getenv("AUTH_SERVICE_PORT")
	log.Printf("Переменная AUTH_SERVICE_PORT загружена из окружения .env: %s", authServicePort)

	httpPort := getEnvOrDefault("ORCHESTRATOR_HTTP_PORT", "8080")
	if httpPort == "" {
		log.Printf("ОШИБКА: Переменная ORCHESTRATOR_HTTP_PORT отсутствует в окружении .env: %s | Используем значение по умолчанию: 8080", httpPort)
		httpPort = "8080"
	}
	grpcPort := getEnvOrDefault("ORCHESTRATOR_GRPC_PORT", "8081")
	if grpcPort == "" {
		log.Printf("ОШИБКА: Переменная ORCHESTRATOR_GRPC_PORT отсутствует в окружении .env: %s | Используем значение по умолчанию: 8081", grpcPort)
		grpcPort = "8081"
	}

	orchestrator.InitTaskManager()
	taskManager := orchestrator.GetTaskManager()

	log.Printf("Сервер запущен с TaskManager: задачи=%d, выражения=%d",
		len(taskManager.GetAllTasks()), len(taskManager.GetAllExpressions()))

	go func() {
		grpcAddress := ":" + grpcPort
		log.Printf("Starting gRPC server for agents on port %s", grpcPort)
		if err := grpc.StartServer(grpcAddress, taskManager); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	r := mux.NewRouter()

	protected := r.PathPrefix("/api/v1").Subrouter()
	protected.Use(orchestrator.AuthMiddleware)

	protected.HandleFunc("/expressions", orchestrator.HandleGetExpressions).Methods("GET")
	protected.HandleFunc("/expressions/{id}", orchestrator.HandleGetExpression).Methods("GET")
	protected.HandleFunc("/calculate", orchestrator.HandleProtectedCalculate).Methods("POST")
	protected.HandleFunc("/history", orchestrator.HandleProtectedHistory).Methods("GET")

	r.HandleFunc("/api/v1/register", orchestrator.HandleRegister).Methods("POST")
	r.HandleFunc("/api/v1/login", orchestrator.HandleLogin).Methods("POST")
	r.HandleFunc("/api/v1/token-info", orchestrator.HandleTokenInfo).Methods("GET")

	staticDir := http.Dir("./cmd/web/static")
	staticFileServer := http.FileServer(staticDir)

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", staticFileServer))
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./cmd/web/static/index.html")
	})

	r.PathPrefix("/").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/static/") {
			http.ServeFile(w, r, "./cmd/web/static/index.html")
		} else {
			http.NotFound(w, r)
		}
	}))

	log.Printf("Starting HTTP server on port %s", httpPort)
	log.Printf("Web interface available on port %s", httpPort)

	if err := http.ListenAndServe(":"+httpPort, r); err != nil {
		log.Fatal(err)
	}
}

func getEnvOrDefault(envVar, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		log.Printf("ОШИБКА: Переменная %s отсутствует в окружении .env: %s | Используем значение по умолчанию: %s", envVar, value, defaultValue)
		return defaultValue
	}
	return value
}
