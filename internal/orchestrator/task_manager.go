package orchestrator

import (
	"errors"
	"gocalc/internal/calculator"
	"gocalc/internal/types"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"gocalc/internal/database"
	"gocalc/internal/models"

	"github.com/google/uuid"
)

var SaveExpressionFunc = database.SaveExpression

// Task представляет задачу для вычисления
type Task struct {
	ID            string  // Уникальный идентификатор задачи
	Arg1          float64 // Первый аргумент
	Arg2          float64 // Второй аргумент
	Operation     string  // Операция: "+", "-", "*", "/"
	OperationTime int     // Время выполнения в мс (для эмуляции нагрузки)
	Priority      int     // Приоритет задачи (1 - низкий, 2 - высокий)
}

type TaskResult struct {
	ID     string
	Result float64
}

type TaskManager struct {
	expressions      map[string]types.Expression
	tasks            map[string]Task
	taskResults      map[string]float64
	taskToExpression map[string]string
	expressionTasks  map[string][]string
	dependsOnTask    map[string][]string
	userIDs          map[string]int
	mu               sync.RWMutex           // Мьютекс для синхронизации
	calc             *calculator.Calculator // Калькулятор для разбора выражений
}

// NewTaskManager создает новый менеджер задач
func NewTaskManager() *TaskManager {
	log.Printf("Загружены параметры времени операций:")
	log.Printf("TIME_ADDITION_MS: %s", os.Getenv("TIME_ADDITION_MS"))
	log.Printf("TIME_SUBTRACTION_MS: %s", os.Getenv("TIME_SUBTRACTION_MS"))
	log.Printf("TIME_MULTIPLICATIONS_MS: %s", os.Getenv("TIME_MULTIPLICATIONS_MS"))
	log.Printf("TIME_DIVISIONS_MS: %s", os.Getenv("TIME_DIVISIONS_MS"))

	return &TaskManager{
		expressions:      make(map[string]types.Expression),
		tasks:            make(map[string]Task),
		taskResults:      make(map[string]float64),
		taskToExpression: make(map[string]string),
		expressionTasks:  make(map[string][]string),
		dependsOnTask:    make(map[string][]string),
		userIDs:          make(map[string]int),
		calc:             calculator.NewCalculator(),
	}
}

// getEnvOrDefaultInt получает значение переменной окружения или возвращает значение по умолчанию
func getEnvOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		} else {
			log.Printf("Ошибка при преобразовании значения переменной %s: %v", key, err)
		}
	} else {
		log.Printf("ВНИМАНИЕ: Переменная окружения %s не установлена, используется значение по умолчанию", key)
	}
	return defaultValue
}

// CreateExpression создает новое выражение и разбивает его на задачи
func (tm *TaskManager) CreateExpression(expressionText string, userID int) (string, error) {
	if expressionText == "" {
		return "", errors.New("empty expression")
	}

	testCalc := calculator.NewCalculator()
	_, err := testCalc.Calculate(expressionText)
	if err != nil {
		errStr := err.Error()
		if errStr == "division by zero" {
			return "", errors.New("division by zero")
		} else if errStr == "unexpected character" || errStr == "syntax error" {
			return "", errors.New("invalid character")
		} else if errStr == "unbalanced parentheses" {
			return "", errors.New("mismatched parentheses")
		} else {
			return "", errors.New("invalid expression")
		}
	}

	// Токенизируем выражение
	if err := tm.calc.Tokenize(expressionText); err != nil {
		return "", errors.New("invalid expression")
	}

	// Преобразуем в RPN
	rpn, err := tm.calc.ToRPN()
	if err != nil {
		return "", errors.New("invalid expression")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(rpn) == 1 && rpn[0].Type == calculator.Number {
		exprID := uuid.New().String()
		result, _ := testCalc.Calculate(expressionText)

		taskID := uuid.New().String()
		task := Task{
			ID:            taskID,
			Arg1:          result,
			Arg2:          0,
			Operation:     "", // Пустая операция для числа
			OperationTime: 0,
			Priority:      1,
		}

		expr := types.Expression{
			ID:        exprID,
			Original:  expressionText,
			Status:    "PROCESSING", // Изменил статус
			Result:    result,
			CreatedAt: time.Now().Format("02.01.2006 15:04:05"),
		}

		log.Printf("ОТЛАДКА: Создано выражение для числа")
		log.Printf("ID выражения: %s", exprID)
		log.Printf("Оригинальное выражение: %s", expressionText)
		log.Printf("Результат: %f", result)
		log.Printf("Статус: %s", expr.Status)
		log.Printf("Время создания: %s", expr.CreatedAt)

		tm.expressions[exprID] = expr
		tm.tasks[taskID] = task
		tm.taskToExpression[taskID] = exprID
		tm.expressionTasks[exprID] = []string{taskID}
		tm.userIDs[exprID] = userID

		return exprID, nil
	}

	// Создаем выражение
	exprID := uuid.New().String()
	expr := types.Expression{
		ID:        exprID,
		Original:  expressionText,
		Status:    "PROCESSING",
		CreatedAt: time.Now().Format("02.01.2006 15:04:05"),
	}
	tm.expressions[exprID] = expr
	tm.userIDs[exprID] = userID

	// Разбиваем выражение на задачи
	var taskIDs []string
	type stackItem struct {
		value  float64
		taskID string
		isNum  bool
	}

	var stack []stackItem

	// Обработка RPN и создание задач
	for _, token := range rpn {
		switch token.Type {
		case calculator.Number:
			num, _ := strconv.ParseFloat(token.Value, 64)
			stack = append(stack, stackItem{
				value: num,
				isNum: true,
			})
		case calculator.Operator:
			if len(stack) < 2 {
				return "", errors.New("invalid expression")
			}

			taskID := uuid.New().String()

			rightOp := stack[len(stack)-1]
			leftOp := stack[len(stack)-2]
			stack = stack[:len(stack)-2]

			task := Task{
				ID:        taskID,
				Operation: token.Value,
			}

			if token.Value == "*" || token.Value == "/" {
				task.Priority = 2
			} else {
				task.Priority = 1
			}

			// Устанавливаем время выполнения операции из переменных окружения
			switch token.Value {
			case "+":
				task.OperationTime = getEnvOrDefaultInt("TIME_ADDITION_MS", 510)
			case "-":
				task.OperationTime = getEnvOrDefaultInt("TIME_SUBTRACTION_MS", 520)
			case "*":
				task.OperationTime = getEnvOrDefaultInt("TIME_MULTIPLICATIONS_MS", 530)
			case "/":
				task.OperationTime = getEnvOrDefaultInt("TIME_DIVISIONS_MS", 540)
			}

			log.Printf("Создана задача %s: операция %s, время выполнения: %d мс",
				taskID, token.Value, task.OperationTime)

			if leftOp.isNum {
				task.Arg1 = leftOp.value
			} else {
				task.Arg1 = 0
				tm.dependsOnTask[taskID] = append(tm.dependsOnTask[taskID], leftOp.taskID)
			}

			if rightOp.isNum {
				task.Arg2 = rightOp.value
			} else {
				task.Arg2 = 0
				tm.dependsOnTask[taskID] = append(tm.dependsOnTask[taskID], rightOp.taskID)
			}

			tm.tasks[taskID] = task
			tm.taskToExpression[taskID] = exprID
			taskIDs = append(taskIDs, taskID)

			stack = append(stack, stackItem{
				taskID: taskID,
				isNum:  false,
			})
		}
	}

	// Проверяем, что задачи были созданы и сохранены в карте задач
	log.Printf("После создания выражения %s количество задач в taskManager: %d", exprID, len(tm.tasks))
	for _, taskID := range taskIDs {
		if _, exists := tm.tasks[taskID]; !exists {
			log.Printf("ОШИБКА: Задача %s не найдена в tm.tasks после создания", taskID)
		} else {
			log.Printf("Задача %s успешно сохранена в tm.tasks", taskID)
		}
	}

	tm.expressionTasks[exprID] = taskIDs
	return exprID, nil
}

// GetNextTask возвращает следующую задачу для вычисления
func (tm *TaskManager) GetNextTask() (Task, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	//log.Printf("GetNextTask: запрошена задача для выполнения. Количество задач: %d", len(tm.tasks))

	// Сначала выбираем задачи с высоким приоритетом
	for id, task := range tm.tasks {
		dependTaskIDs, hasDependency := tm.dependsOnTask[id]

		// Если нет зависимостей - возвращаем задачу
		if !hasDependency || len(dependTaskIDs) == 0 {
			log.Printf("Подготовка задачи %s для распределения. Нет зависимостей.", id)
			delete(tm.tasks, id)
			return task, true
		}

		// Проверяем, все ли зависимости выполнены
		allDepsDone := true
		for _, depID := range dependTaskIDs {
			if _, ok := tm.taskResults[depID]; !ok {
				allDepsDone = false
				break
			}
		}
		if allDepsDone {
			log.Printf("Подготовка задачи %s. Все зависимости выполнены.", id)
			depIdx := 0
			if task.Arg1 == 0 && depIdx < len(dependTaskIDs) {
				task.Arg1 = tm.taskResults[dependTaskIDs[depIdx]]
				depIdx++
			}
			if task.Arg2 == 0 && depIdx < len(dependTaskIDs) {
				task.Arg2 = tm.taskResults[dependTaskIDs[depIdx]]
			}
			delete(tm.tasks, id)
			delete(tm.dependsOnTask, id)
			return task, true
		}
	}

	return Task{}, false
}

// SubmitTaskResult обрабатывает результат вычисления
func (tm *TaskManager) SubmitTaskResult(result TaskResult) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	log.Printf("Получен результат задачи %s: %f", result.ID, result.Result)

	tm.taskResults[result.ID] = result.Result

	exprID, exists := tm.taskToExpression[result.ID]
	if !exists {
		log.Printf("ОШИБКА: Задача %s не найдена в taskToExpression", result.ID)
		return errors.New("задача не найдена")
	}

	log.Printf("Задача %s связана с выражением %s", result.ID, exprID)

	taskIDs, ok := tm.expressionTasks[exprID]
	if !ok {
		log.Printf("ОШИБКА: Для выражения %s не найдены связанные задачи", exprID)
		return errors.New("задачи выражения не найдены")
	}

	// Проверяем, все ли задачи выполнены
	allTasksCompleted := true
	completedTasks := 0
	for _, taskID := range taskIDs {
		if _, ok := tm.taskResults[taskID]; ok {
			completedTasks++
			log.Printf("Задача %s для выражения %s выполнена", taskID, exprID)
		} else {
			allTasksCompleted = false
			log.Printf("Задача %s для выражения %s еще не выполнена", taskID, exprID)
		}
	}

	log.Printf("Для выражения %s выполнено %d/%d задач", exprID, completedTasks, len(taskIDs))

	log.Printf("Зависимости задач для выражения %s:", exprID)
	if allTasksCompleted {
		log.Printf("Все задачи для выражения %s выполнены, обновляем статус", exprID)

		expr, exists := tm.expressions[exprID]
		if !exists {
			log.Printf("ОШИБКА: Выражение %s не найдено в списке выражений", exprID)
			return errors.New("выражение не найдено")
		}

		log.Printf("Текущее состояние выражения %s: статус=%s, результат=%f", exprID, expr.Status, expr.Result)

		var rootTaskID string
		isRoot := make(map[string]bool)
		for _, taskID := range taskIDs {
			isRoot[taskID] = true
		}

		for _, dependIDs := range tm.dependsOnTask {
			for _, dependID := range dependIDs {
				isRoot[dependID] = false
			}
		}

		log.Printf("isRoot карта для выражения %s: %v", exprID, isRoot)

		for _, taskID := range taskIDs {
			if isRoot[taskID] {
				rootTaskID = taskID
				log.Printf("Задача %s определена как корневая для выражения %s", taskID, exprID)
			}
		}

		userID := tm.userIDs[exprID]

		if rootTaskID != "" {
			finalResult := tm.taskResults[rootTaskID]
			log.Printf("Используем результат корневой задачи %s: %f", rootTaskID, finalResult)

			expr.Status = "COMPLETED"
			expr.Result = finalResult
			tm.expressions[exprID] = expr

			// Сохраняем в БД
			dbExpr := models.Expression{
				ID:        expr.ID,
				Text:      expr.Original,
				Status:    expr.Status,
				Result:    expr.Result,
				CreatedAt: expr.CreatedAt,
			}
			_ = SaveExpressionFunc(&dbExpr, userID)

			log.Printf("Выражение %s (%s) вычислено с учетом временных задержек операций. Итоговый результат: %f", exprID, expr.Original, finalResult)
		} else {
			lastTaskID := taskIDs[len(taskIDs)-1]
			finalResult := tm.taskResults[lastTaskID]
			log.Printf("Корневая задача не найдена, используем последнюю задачу %s: %f", lastTaskID, finalResult)

			expr.Status = "COMPLETED"
			expr.Result = finalResult
			tm.expressions[exprID] = expr

			// Сохраняем в БД
			dbExpr := models.Expression{
				ID:        expr.ID,
				Text:      expr.Original,
				Status:    expr.Status,
				Result:    expr.Result,
				CreatedAt: expr.CreatedAt,
			}
			_ = SaveExpressionFunc(&dbExpr, userID)

			log.Printf("Выражение %s (%s) вычислено с использованием последней задачи. Итоговый результат: %f", exprID, expr.Original, finalResult)
		}

		log.Printf("Обновлено выражение %s: статус=%s, результат=%f", exprID, expr.Status, expr.Result)

		// Очищаем данные о выполненных задачах
		for _, taskID := range taskIDs {
			delete(tm.taskResults, taskID)
			delete(tm.taskToExpression, taskID)
			delete(tm.dependsOnTask, taskID)
			delete(tm.tasks, taskID)
		}
		delete(tm.expressionTasks, exprID)
		log.Printf("Очищены данные о задачах для выражения %s", exprID)
	}

	return nil
}

func (tm *TaskManager) GetExpression(id string) (types.Expression, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	expr, exists := tm.expressions[id]
	return expr, exists
}

func (tm *TaskManager) GetAllExpressions() []types.Expression {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []types.Expression
	for _, expr := range tm.expressions {
		result = append(result, expr)
	}

	for i, expr := range result {
		log.Printf("DEBUG: Выражение %d: ID=%s, Оригинал=%s, Статус=%s, Результат=%f, Создано=%s",
			i, expr.ID, expr.Original, expr.Status, expr.Result, expr.CreatedAt)
	}

	return result
}

func (tm *TaskManager) GetAllTasks() []Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []Task
	for _, task := range tm.tasks {
		result = append(result, task)
	}
	return result
}

// ResetState сбрасывает состояние менеджера
func (tm *TaskManager) ResetState() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.expressions = make(map[string]types.Expression)
	tm.tasks = make(map[string]Task)
	tm.taskResults = make(map[string]float64)
	tm.taskToExpression = make(map[string]string)
	tm.expressionTasks = make(map[string][]string)
	tm.dependsOnTask = make(map[string][]string)
	tm.userIDs = make(map[string]int)
	tm.calc = calculator.NewCalculator()
}

// GetUserExpressions возвращает все выражения конкретного пользователя
func (tm *TaskManager) GetUserExpressions(userID int) []types.Expression {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []types.Expression
	for exprID, expr := range tm.expressions {
		if storedUserID, exists := tm.userIDs[exprID]; exists && storedUserID == userID {
			result = append(result, expr)
		}
	}

	return result
}

func (tm *TaskManager) SetUserIDForExpression(exprID string, userID int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.userIDs[exprID] = userID
}
