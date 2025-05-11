package orchestrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"gocalc/internal/auth"
	"gocalc/internal/calculator"
	"gocalc/internal/database"
	"gocalc/internal/models"
	"gocalc/internal/types"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HandleAuthProxy перенаправляет все запросы авторизации на сервис авторизации
func HandleAuthProxy(w http.ResponseWriter, r *http.Request) {
	// URL сервиса авторизации
	authServicePort := getEnvOrDefault("AUTH_SERVICE_PORT", "8083")
	authServiceURL := "http://localhost:" + authServicePort

	var requestBody []byte
	var err error

	if r.Body != nil {
		requestBody, err = io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Ошибка при чтении тела запроса: %v", err)
			http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
			return
		}
		// Закрываем исходное тело
		r.Body.Close()
		// Создаем новое тело с прочитанными данными
		r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	}

	// Создаем URL для прокси
	target, err := url.Parse(authServiceURL)
	if err != nil {
		log.Printf("Ошибка при разборе URL сервиса авторизации: %v", err)
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	// Создаем новый реверс-прокси
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Сохраняем исходный обработчик директора
	originalDirector := proxy.Director

	// Создаем новый обработчик директора, который сохраняет путь запроса
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Сохраняем оригинальный путь запроса
		req.URL.Path = r.URL.Path
		// Добавляем все параметры запроса
		req.URL.RawQuery = r.URL.RawQuery
	}

	// Обновляем заголовок Host для соответствия целевому серверу
	r.Host = target.Host

	// Выполняем проксирование запроса
	proxy.ServeHTTP(w, r)
}

// HandleRegister перенаправляет запросы регистрации на сервис авторизации
func HandleRegister(w http.ResponseWriter, r *http.Request) {
	HandleAuthProxy(w, r)
}

// HandleLogin перенаправляет запросы авторизации на сервис авторизации
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	HandleAuthProxy(w, r)
}

// HandleTokenInfo перенаправляет запросы информации о токене на сервис авторизации
func HandleTokenInfo(w http.ResponseWriter, r *http.Request) {
	HandleAuthProxy(w, r)
}

// HandleProtectedCalculate перенаправляет запросы вычислений с авторизацией на сервис авторизации
func HandleProtectedCalculate(w http.ResponseWriter, r *http.Request) {
	// Проверяем наличие заголовка авторизации
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	// Получаем токен JWT
	token := strings.TrimPrefix(authHeader, "Bearer ")

	// Валидация токена и извлечение userID
	claims, err := auth.ValidateToken(token)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}
	userID := claims.UserID

	// Токен действителен, теперь обрабатываем выражение локально через оркестратор
	log.Println("Токен действителен, начинаем вычисление выражения через оркестратор-агент")

	// Читаем тело запроса с выражением
	var calcReq CalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&calcReq); err != nil {
		log.Printf("Ошибка при декодировании запроса расчета: %v", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	log.Printf("Вызываем локальную обработку выражения: %s", calcReq.Expression)
	log.Printf("RAW expression bytes: %v", []byte(calcReq.Expression))
	log.Printf("RAW expression string: %q", calcReq.Expression)

	var invalidExprError error
	if calcReq.Expression == "" {
		invalidExprError = errors.New("empty expression")
	} else {
		_, calcErr := calculator.Calc(calcReq.Expression)
		if calcErr != nil {
			invalidExprError = calcErr
		}
	}

	if invalidExprError != nil {
		exprID := uuid.New().String()

		expr := types.Expression{
			ID:        exprID,
			Original:  calcReq.Expression,
			Status:    "error",
			Result:    0,
			CreatedAt: time.Now().Format("02.01.2006 15:04:05"),
		}

		// Сохраняем выражение в менеджере задач
		GetTaskManager().mu.Lock()
		GetTaskManager().expressions[exprID] = expr
		GetTaskManager().mu.Unlock()

		// Сохраняем выражение с ошибкой в БД
		dbExpr := models.Expression{
			ID:        expr.ID,
			Text:      expr.Original,
			Status:    expr.Status,
			Result:    expr.Result,
			CreatedAt: expr.CreatedAt,
		}
		_ = database.SaveExpression(&dbExpr, userID)

		errMsg := invalidExprError.Error()

		if strings.Contains(errMsg, "invalid character") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid expression: invalid character"})
			return
		}

		if strings.Contains(errMsg, "empty expression") ||
			strings.Contains(errMsg, "invalid expression") ||
			strings.Contains(errMsg, "division by zero") ||
			strings.Contains(errMsg, "mismatched parentheses") ||
			strings.Contains(errMsg, "tokenization error") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid expression: " + errMsg})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error processing expression: " + errMsg})
		return
	}

	// Если выражение валидно, обрабатываем через оркестратор-агент
	exprID, err := GetTaskManager().CreateExpression(calcReq.Expression, userID)
	if err != nil {
		log.Printf("Ошибка при создании выражения: %v", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error processing expression: " + err.Error()})
		return
	}

	log.Printf("Выражение создано: ID=%s, начинается вычисление с использованием агентов", exprID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": exprID})
}

// HandleProtectedHistory перенаправляет запросы истории с авторизацией на сервис авторизации
func HandleProtectedHistory(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	authServicePort := getEnvOrDefault("AUTH_SERVICE_PORT", "8083")
	authServiceURL := "http://localhost:" + authServicePort

	target, err := url.Parse(authServiceURL)
	if err != nil {
		log.Printf("Ошибка при разборе URL сервиса авторизации: %v", err)
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	// Создаем специальный клиент для истории, чтобы адаптировать формат ответа
	client := &http.Client{}

	// Клонируем запрос
	proxyReq, err := http.NewRequest(r.Method, authServiceURL+r.URL.Path, r.Body)
	if err != nil {
		log.Printf("Ошибка при создании запроса прокси: %v", err)
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	// Копируем заголовки
	for header, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(header, value)
		}
	}

	// Устанавливаем правильный хост
	proxyReq.Host = target.Host

	// Выполняем запрос
	resp, err := client.Do(proxyReq)
	if err != nil {
		//log.Printf("Ошибка при выполнении запроса прокси: %v", err)
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Ошибка при чтении тела ответа: %v", err)
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if resp.StatusCode == http.StatusOK {
		var objData map[string]interface{}
		if err := json.Unmarshal(body, &objData); err == nil {
			if _, ok := objData["expressions"]; ok {
				w.WriteHeader(http.StatusOK)
				w.Write(body)
				return
			}
		}

		var arrData []interface{}
		if err := json.Unmarshal(body, &arrData); err == nil {
			log.Printf("Данные истории получены как массив, длина: %d", len(arrData))

			// Это массив выражений, нужно обернуть в объект с ключом expressions
			log.Printf("Преобразуем массив в формат с expressions")
			// Создаем новую структуру с правильным форматом
			newResponse := map[string]interface{}{
				"expressions": arrData,
			}

			// Преобразуем в JSON и отправляем
			jsonResponse, err := json.Marshal(newResponse)
			if err != nil {
				log.Printf("Ошибка при маршалинге JSON: %v", err)
				http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write(jsonResponse)
			return
		}

		// Ни один формат не подошел, пробуем обработать объект особым образом
		if objData != nil {
			// Проверяем, если в objData есть expressions, но это не массив
			if expData, ok := objData["expressions"]; ok {
				// Это может быть объект, пробуем преобразовать в массив
				log.Printf("Ключ expressions найден, но требует дополнительной обработки: %T", expData)

				switch v := expData.(type) {
				case []interface{}:
					// Это уже массив, все хорошо
					log.Printf("expressions уже является массивом длиной %d", len(v))
				case map[string]interface{}:
					// Это объект, преобразуем его в массив
					log.Printf("expressions является объектом, преобразуем в массив")
					var expressionsArray []interface{}
					for _, item := range v {
						expressionsArray = append(expressionsArray, item)
					}
					objData["expressions"] = expressionsArray

					jsonResponse, err := json.Marshal(objData)
					if err != nil {
						log.Printf("Ошибка при маршалинге JSON: %v", err)
						http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(jsonResponse)
					return
				default:
					// Неизвестный тип, создаем пустой массив
					log.Printf("expressions имеет неизвестный тип %T, создаем пустой массив", expData)
					objData["expressions"] = []interface{}{}

					jsonResponse, err := json.Marshal(objData)
					if err != nil {
						log.Printf("Ошибка при маршалинге JSON: %v", err)
						http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
						return
					}

					w.WriteHeader(http.StatusOK)
					w.Write(jsonResponse)
					return
				}
			} else {
				// В объекте нет ключа expressions, оборачиваем весь объект в массив
				log.Printf("Объект без ключа expressions, оборачиваем весь объект в expressions")
				newResponse := map[string]interface{}{
					"expressions": []interface{}{objData},
				}

				jsonResponse, err := json.Marshal(newResponse)
				if err != nil {
					log.Printf("Ошибка при маршалинге JSON: %v", err)
					http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write(jsonResponse)
				return
			}
		}

		// Если все проверки не сработали, создаем структуру с пустым массивом expressions
		log.Printf("Создаем структуру с пустым массивом expressions")
		emptyResponse := map[string]interface{}{
			"expressions": []interface{}{},
		}

		jsonResponse, err := json.Marshal(emptyResponse)
		if err != nil {
			log.Printf("Ошибка при маршалинге JSON: %v", err)
			http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(jsonResponse)
		return
	}

	// Если получили не 200 OK, просто пересылаем ответ как есть
	for header, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(header, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// Вспомогательная функция для чтения переменной окружения с дефолтом
func getEnvOrDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}
