package orchestrator

import (
	"encoding/json"
	"gocalc/internal/calculator"
	"gocalc/internal/types"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gorilla/mux"
)

var (
	expressions      = make(map[string]types.Expression)
	tasks            = make(map[string]types.Task)
	taskResults      = make(map[string]float64)
	taskToExpression = make(map[string]string)
	expressionTasks  = make(map[string][]string)
	dependsOnTask    = make(map[string][]string)
	mu               sync.RWMutex
	calc             = calculator.NewCalculator()
	taskManager      *TaskManager
)

type stackItem struct {
	value  float64
	taskID string
	isNum  bool
}

func ResetState() {
	mu.Lock()
	defer mu.Unlock()

	expressions = make(map[string]types.Expression)
	tasks = make(map[string]types.Task)
	taskResults = make(map[string]float64)
	taskToExpression = make(map[string]string)
	expressionTasks = make(map[string][]string)
	dependsOnTask = make(map[string][]string)
	calc = calculator.NewCalculator()

	if taskManager != nil {
		taskManager.ResetState()
	}

	taskManager = NewTaskManager()
}

func InitTaskManager() {
	log.Printf("Инициализация менеджера задач")
	taskManager = NewTaskManager()
}

func GetTaskManager() *TaskManager {
	if taskManager == nil {
		log.Printf("ВНИМАНИЕ: taskManager был nil, создаем новый экземпляр")
		InitTaskManager()
	}
	return taskManager
}

type CalculateRequest struct {
	Expression string `json:"expression"`
}

func HandleCalculate(w http.ResponseWriter, r *http.Request) {
	// Проверка аутентификации
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Получаем userID из контекста
	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var calcReq CalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&calcReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	calcReq.Expression = strings.TrimSpace(calcReq.Expression)

	log.Printf("Токен действителен, начинаем вычисление выражения через оркестратор-агент")
	log.Printf("Вызываем локальную обработку выражения: %s", calcReq.Expression)

	// Создаем выражение в TaskManager
	exprID, err := GetTaskManager().CreateExpression(calcReq.Expression, userID)
	if err != nil {
		log.Printf("Ошибка создания выражения: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Получаем созданное выражение
	expr, exists := GetTaskManager().GetExpression(exprID)
	if !exists {
		log.Printf("Созданное выражение не найдено: %s", exprID)
		http.Error(w, "Не удалось создать выражение", http.StatusInternalServerError)
		return
	}

	// Сохраняем выражение в глобальной переменной
	mu.Lock()
	expressions[exprID] = expr
	mu.Unlock()

	log.Printf("Выражение создано: ID=%s, начинается вычисление", exprID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"id":         exprID,
		"expression": calcReq.Expression,
		"status":     "PROCESSING",
	})
}

func HandleGetExpressions(w http.ResponseWriter, r *http.Request) {
	log.Printf("HandleGetExpressions: старт обработки запроса /api/v1/expressions")

	// Получаем userID из контекста
	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	taskManager := GetTaskManager()

	log.Printf("ОТЛАДКА: Состояние TaskManager перед получением выражений:")
	log.Printf("Количество выражений в taskManager: %d", len(taskManager.expressions))
	log.Printf("Количество выражений в глобальной переменной: %d", len(expressions))
	log.Printf("Количество задач: %d", len(taskManager.tasks))
	log.Printf("Количество результатов задач: %d", len(taskManager.taskResults))

	mu.RLock()
	log.Printf("HandleGetExpressions: чтение выражений из глобальной переменной")
	var globalExpressions []types.Expression
	for exprID, expr := range expressions {
		if exprUserID, exists := taskManager.userIDs[exprID]; exists && exprUserID == userID {
			globalExpressions = append(globalExpressions, expr)
		}
	}
	mu.RUnlock()

	log.Printf("HandleGetExpressions: чтение выражений из TaskManager для пользователя %d", userID)
	managerExpressions := taskManager.GetUserExpressions(userID)

	log.Printf("HandleGetExpressions: объединение выражений (глобальные + менеджер)")
	allExpressions := append(globalExpressions, managerExpressions...)

	log.Printf("HandleGetExpressions: сортировка выражений по дате создания")
	sort.Slice(allExpressions, func(i, j int) bool {
		return allExpressions[i].CreatedAt > allExpressions[j].CreatedAt
	})

	// Создаем ответ с явным указанием пустого массива, если нет выражений
	log.Printf("HandleGetExpressions: формирование ответа для клиента")
	response := types.ExpressionResponse{
		Expressions: allExpressions,
	}

	w.Header().Set("Content-Type", "application/json")

	// Принудительно сериализуем с пустым массивом, если nil
	if response.Expressions == nil {
		response.Expressions = []types.Expression{}
	}

	// Логируем для отладки
	log.Printf("ОТЛАДКА: Возвращаю список выражений. Количество: %d", len(response.Expressions))

	// Дополнительное логирование каждого выражения
	for i, expr := range response.Expressions {
		log.Printf("ОТЛАДКА: Выражение %d: ID=%s, Оригинал=%s, Статус=%s, Результат=%f, Создано=%s",
			i, expr.ID, expr.Original, expr.Status, expr.Result, expr.CreatedAt)
	}

	// Принудительная сериализация с обработкой ошибок
	log.Printf("HandleGetExpressions: сериализация и отправка ответа клиенту")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ") // Для читаемости
	if err := encoder.Encode(response); err != nil {
		log.Printf("ОШИБКА сериализации: %v", err)
		http.Error(w, "Ошибка сериализации", http.StatusInternalServerError)
	}
	log.Printf("HandleGetExpressions: завершение обработки запроса /api/v1/expressions")
}

func HandleGetExpression(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	expr, exists := GetTaskManager().GetExpression(id)
	if !exists {
		log.Printf("Выражение с ID %s не найдено", id)
		http.Error(w, "Expression not found", http.StatusNotFound)
		return
	}

	// Проверяем, принадлежит ли выражение пользователю
	taskManager := GetTaskManager()
	if exprUserID, exists := taskManager.userIDs[id]; !exists || exprUserID != userID {
		log.Printf("Выражение с ID %s не принадлежит пользователю %d", id, userID)
		http.Error(w, "Expression not found", http.StatusNotFound)
		return
	}

	log.Printf("Найдено выражение: ID=%s, статус=%s, оригинал=%s, результат=%f",
		expr.ID, expr.Status, expr.Original, expr.Result)

	w.Header().Set("Content-Type", "application/json")

	// Сериализуем данные в JSON и проверяем содержимое
	jsonData, err := json.Marshal(expr)
	if err != nil {
		log.Printf("Ошибка сериализации выражения в JSON: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("Отправляемые данные: %s", string(jsonData))
	w.Write(jsonData)
}

func HandleGetTask(w http.ResponseWriter, r *http.Request) {
	task, found := GetTaskManager().GetNextTask()
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func HandleSubmitTaskResult(w http.ResponseWriter, r *http.Request) {
	var result types.TaskResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Конвертируем types.TaskResult в orchestrator.TaskResult
	taskResult := TaskResult{
		ID:     result.ID,
		Result: result.Result,
	}

	err := GetTaskManager().SubmitTaskResult(taskResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Получаем связанное выражение
	taskManager := GetTaskManager()
	exprID, exists := taskManager.taskToExpression[result.ID]
	if exists {
		// Обновляем выражение в глобальной переменной
		expr, exprExists := taskManager.GetExpression(exprID)
		if exprExists {
			mu.Lock()
			expressions[exprID] = expr
			mu.Unlock()
		}
	}

	w.WriteHeader(http.StatusOK)
}

// getUserIDFromContext извлекает userID из контекста запроса
func getUserIDFromContext(ctx interface{}) (int, bool) {
	c, ok := ctx.(interface {
		Value(key interface{}) interface{}
	})
	if !ok {
		return 0, false
	}
	userID, ok := c.Value("userID").(int)
	return userID, ok
}
