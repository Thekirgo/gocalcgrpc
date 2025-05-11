package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"gocalc/internal/auth"
	"gocalc/internal/calculator"
	"gocalc/internal/database"
	"gocalc/internal/models"
)

type TokenInfoResponse struct {
	ExpirationMinutes string `json:"expirationMinutes"`
}

type CalculateRequest struct {
	Expression string `json:"expression"`
}

type CalculateResponse struct {
	Result string `json:"result"`
}

type AuthRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token  string `json:"token"`
	Status string `json:"status"`
}

func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func main() {
	log.Printf("Время жизни токена: %d минут", auth.GetTokenExpiration())
	db := database.GetDB()
	defer db.Close()

	// Создаем экземпляр калькулятора
	calc := calculator.NewCalculator()

	// Настраиваем маршруты API с использованием mux
	r := mux.NewRouter()

	// Обслуживание статических файлов
	webDir := filepath.Join("cmd", "web", "static")

	// Корневой маршрут перенаправляет на index.html
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(webDir))))

	apiV1 := r.PathPrefix("/api/v1").Subrouter()
	apiV1.HandleFunc("/token-info", tokenInfoHandler).Methods(http.MethodGet)
	apiV1.HandleFunc("/register", registerHandler).Methods(http.MethodPost)
	apiV1.HandleFunc("/login", loginHandler).Methods(http.MethodPost)

	protected := apiV1.PathPrefix("/").Subrouter()
	protected.Use(authMiddleware)

	protected.HandleFunc("/calculate", func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserIDFromContext(r.Context())
		if !ok {
			sendJSONError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		// Декодируем запрос
		var req CalculateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSONError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		// Проверяем, что выражение не пустое
		if req.Expression == "" {
			sendJSONError(w, http.StatusBadRequest, "Выражение не может быть пустым")
			return
		}

		expressionID := uuid.New().String()

		// Создаем объект выражения для сохранения в БД
		expression := models.Expression{
			ID:     expressionID,
			Text:   req.Expression,
			Status: "processing",
		}

		// Вычисляем результат
		result, err := calc.Calculate(req.Expression)
		if err != nil {
			expression.Status = "error"
			_ = database.SaveExpression(&expression, userID)

			if strings.Contains(err.Error(), "invalid") ||
				strings.Contains(err.Error(), "tokenization error") ||
				strings.Contains(err.Error(), "division by zero") ||
				strings.Contains(err.Error(), "mismatched parentheses") {

				errorMsg := err.Error()
				errorMsg = strings.Replace(errorMsg, "tokenization error: ", "", 1)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("Expression is not valid: %v", errorMsg),
				})
				return
			}

			// Для других ошибок используем стандартный ответ с ошибкой
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := CalculateResponse{
				Result: fmt.Sprintf("Ошибка: %v", err),
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Обновляем статус и результат выражения
		expression.Status = "completed"
		expression.Result = result

		// Сохраняем выражение с результатом
		err = database.SaveExpression(&expression, userID)
		if err != nil {
			log.Printf("Ошибка сохранения выражения: %v", err)
		}

		// Отправляем результат
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := CalculateResponse{
			Result: fmt.Sprintf("%g", result),
		}
		json.NewEncoder(w).Encode(response)
	}).Methods(http.MethodPost)

	protected.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
		// Получаем ID пользователя из контекста
		userID, ok := getUserIDFromContext(r.Context())
		if !ok {
			sendJSONError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		// Получаем историю выражений из БД
		expressions, err := database.GetExpressions(userID)
		if err != nil {
			log.Printf("Ошибка получения истории: %v", err)
			sendJSONError(w, http.StatusInternalServerError, "Не удалось получить историю")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"expressions": expressions,
		})
	}).Methods(http.MethodGet)

	// Определяем порт для сервера
	port := getEnvOrDefault("CALC_SERVICE_PORT", "8082")
	addr := fmt.Sprintf(":%s", port)

	// Запускаем сервер
	log.Printf("Сервер запущен на http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(addr, r))
}

// Обработчик для получения информации о времени жизни токена
func tokenInfoHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем время жизни токена из пакета auth
	expirationMinutes := auth.GetTokenExpiration()

	// Подготавливаем ответ
	response := TokenInfoResponse{
		ExpirationMinutes: strconv.Itoa(expirationMinutes),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Неверный формат запроса")
		return
	}

	if req.Login == "" || req.Password == "" {
		sendJSONError(w, http.StatusBadRequest, "Логин и пароль не могут быть пустыми")
		return
	}

	// Создаем пользователя в базе данных
	_, err := database.CreateUser(req.Login, req.Password)
	if err != nil {
		log.Printf("Ошибка при регистрации пользователя: %v", err)
		if strings.Contains(err.Error(), "уже существует") {
			sendJSONError(w, http.StatusBadRequest, "Пользователь с таким логином уже существует")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "Ошибка при регистрации")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// Обработчик для авторизации пользователя
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Неверный формат запроса")
		return
	}

	if req.Login == "" || req.Password == "" {
		sendJSONError(w, http.StatusBadRequest, "Логин и пароль не могут быть пустыми")
		return
	}

	user, err := database.GetUser(req.Login)
	if err != nil {
		log.Printf("Ошибка при получении пользователя: %v", err)
		sendJSONError(w, http.StatusInternalServerError, "Ошибка авторизации")
		return
	}

	if user == nil {
		sendJSONError(w, http.StatusUnauthorized, "Неверные учетные данные")
		return
	}

	// Проверяем пароль
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		sendJSONError(w, http.StatusUnauthorized, "Неверные учетные данные")
		return
	}

	// Генерируем JWT токен
	token, err := auth.GenerateToken(user.ID, user.Login)
	if err != nil {
		log.Printf("Ошибка генерации токена: %v", err)
		sendJSONError(w, http.StatusInternalServerError, "Ошибка авторизации")
		return
	}

	// Отправляем ответ с токеном
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := LoginResponse{
		Token:  token,
		Status: "success",
	}
	json.NewEncoder(w).Encode(response)
}

// Middleware для проверки JWT токена
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			sendJSONError(w, http.StatusUnauthorized, "Отсутствует токен авторизации")
			return
		}

		// Токен должен быть в формате "Bearer {token}"
		if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			sendJSONError(w, http.StatusUnauthorized, "Неверный формат токена")
			return
		}

		// Извлекаем токен
		tokenString := authHeader[7:]

		// Проверяем и извлекаем данные из токена
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			sendJSONError(w, http.StatusUnauthorized, "Неверный или просроченный токен")
			return
		}

		// Добавляем ID пользователя в контекст запроса
		ctx := r.Context()
		ctx = setUserIDInContext(ctx, claims.UserID)

		// Вызываем следующий обработчик с обновленным контекстом
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Вспомогательные функции для работы с контекстом
type contextKey string

const userIDKey contextKey = "userID"

func setUserIDInContext(ctx context.Context, userID int) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func getUserIDFromContext(ctx context.Context) (int, bool) {
	userID, ok := ctx.Value(userIDKey).(int)
	return userID, ok
}

// getEnvOrDefault возвращает значение переменной окружения или значение по умолчанию, если переменная не найдена
func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
