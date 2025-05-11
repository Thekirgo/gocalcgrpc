package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"gocalc/internal/calculator"
	"gocalc/internal/orchestrator"
	"gocalc/internal/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func setupTest() {
	orchestrator.ResetState()
}

// Мок-авторизация с проверкой Bearer-токена
func mockAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем наличие заголовка авторизации в тестовых запросах
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Добавляем userID в контекст, как это делает реальный middleware
		ctx := context.WithValue(r.Context(), "userID", 1)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func prepareRouter() *mux.Router {
	router := mux.NewRouter()

	// Маршруты, требующие авторизации
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(mockAuthMiddleware)
	apiRouter.HandleFunc("/calculate", orchestrator.HandleCalculate).Methods("POST")
	apiRouter.HandleFunc("/expressions", orchestrator.HandleGetExpressions).Methods("GET")
	apiRouter.HandleFunc("/expressions/{id}", orchestrator.HandleGetExpression).Methods("GET")

	// Маршруты, не требующие авторизации
	router.HandleFunc("/internal/task", orchestrator.HandleGetTask).Methods("GET")
	router.HandleFunc("/internal/task", orchestrator.HandleSubmitTaskResult).Methods("POST")

	return router
}

func TestHandleGetExpressions(t *testing.T) {
	setupTest()

	router := prepareRouter()

	calcReq := httptest.NewRequest(http.MethodPost, "/api/v1/calculate", strings.NewReader(`{"expression": "2+2*2"}`))
	calcReq.Header.Set("Content-Type", "application/json")
	calcReq.Header.Set("Authorization", "Bearer test-token")
	calcW := httptest.NewRecorder()
	router.ServeHTTP(calcW, calcReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/expressions", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HandleGetExpressions() код статуса = %v, ожидается %v", w.Code, http.StatusOK)
	}

	var response types.ExpressionResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Невозможно распарсить ответ: %v", err)
	}

	if response.Expressions == nil {
		t.Errorf("HandleGetExpressions() должен вернуть список выражений, даже если он пустой")
	}
}

func TestHandleGetTask(t *testing.T) {
	tests := []struct {
		name             string
		expression       string
		expectedPriority int
		expectedOp       string
	}{
		{
			name:             "приоритет умножения",
			expression:       "2+2*3",
			expectedPriority: 2,
			expectedOp:       "*",
		},
		{
			name:             "приоритет деления",
			expression:       "2+6/3",
			expectedPriority: 2,
			expectedOp:       "/",
		},
		{
			name:             "приоритет сложения после умножения",
			expression:       "2*3+4",
			expectedPriority: 2,
			expectedOp:       "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTest()

			router := prepareRouter()

			calcReq := httptest.NewRequest(http.MethodPost, "/api/v1/calculate",
				strings.NewReader(`{"expression": "`+tt.expression+`"}`))
			calcReq.Header.Set("Content-Type", "application/json")
			calcReq.Header.Set("Authorization", "Bearer test-token")
			calcW := httptest.NewRecorder()
			router.ServeHTTP(calcW, calcReq)

			req := httptest.NewRequest(http.MethodGet, "/internal/task", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusNoContent {
				return
			}

			if w.Code != http.StatusOK {
				t.Errorf("HandleGetTask() код статуса = %v, ожидается %v", w.Code, http.StatusOK)
			}

			var task types.Task
			err := json.Unmarshal(w.Body.Bytes(), &task)
			if err != nil {
				t.Fatalf("Невозможно распарсить ответ: %v", err)
			}

			if task.Priority != tt.expectedPriority {
				t.Errorf("HandleGetTask() priority = %v, ожидается %v", task.Priority, tt.expectedPriority)
			}
			if task.Operation != tt.expectedOp {
				t.Errorf("HandleGetTask() operation = %v, ожидается %v", task.Operation, tt.expectedOp)
			}
		})
	}
}

func TestHandleSubmitTaskResult(t *testing.T) {
	setupTest()

	router := prepareRouter()

	calcReq := httptest.NewRequest(http.MethodPost, "/api/v1/calculate",
		strings.NewReader(`{"expression": "2*3"}`))
	calcReq.Header.Set("Content-Type", "application/json")
	calcReq.Header.Set("Authorization", "Bearer test-token")
	calcW := httptest.NewRecorder()
	router.ServeHTTP(calcW, calcReq)

	taskReq := httptest.NewRequest(http.MethodGet, "/internal/task", nil)
	taskW := httptest.NewRecorder()
	router.ServeHTTP(taskW, taskReq)

	var task types.Task
	err := json.Unmarshal(taskW.Body.Bytes(), &task)
	if err != nil || task.ID == "" {
		return
	}

	taskResult := types.TaskResult{
		ID:     task.ID,
		Result: 6, // 2*3 = 6
	}
	resultBody, _ := json.Marshal(taskResult)

	resultReq := httptest.NewRequest(http.MethodPost, "/internal/task", bytes.NewReader(resultBody))
	resultReq.Header.Set("Content-Type", "application/json")
	resultW := httptest.NewRecorder()
	router.ServeHTTP(resultW, resultReq)

	if resultW.Code != http.StatusOK {
		t.Errorf("HandleSubmitTaskResult() код статуса = %v, ожидается %v", resultW.Code, http.StatusOK)
	}

	var calcResponse map[string]string
	err = json.Unmarshal(calcW.Body.Bytes(), &calcResponse)
	if err != nil {
		t.Fatalf("Невозможно распарсить ответ: %v", err)
	}
	exprID := calcResponse["id"]

	tm := orchestrator.GetTaskManager()
	tm.SetUserIDForExpression(exprID, 1)

	exprReq := httptest.NewRequest(http.MethodGet, "/api/v1/expressions/"+exprID, nil)
	exprReq = mux.SetURLVars(exprReq, map[string]string{"id": exprID})
	exprReq.Header.Set("Authorization", "Bearer test-token")
	exprW := httptest.NewRecorder()
	router.ServeHTTP(exprW, exprReq)

	var expr types.Expression
	err = json.Unmarshal(exprW.Body.Bytes(), &expr)
	if err != nil {
		t.Fatalf("Невозможно распарсить ответ: %v", err)
	}

	if expr.Status != "COMPLETED" {
		t.Errorf("После отправки результата статус должен быть COMPLETED, получено %v", expr.Status)
	}
	if expr.Result != 6 {
		t.Errorf("Результат должен быть 6, получено %v", expr.Result)
	}
}

func TestOrderOfOperations(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
	}{
		{
			name:     "сложение и умножение",
			expr:     "2+3*4",
			expected: 14, // 2+(3*4)
		},
		{
			name:     "умножение и сложение",
			expr:     "2*3+4",
			expected: 10, // (2*3)+4
		},
		{
			name:     "вычитание и умножение",
			expr:     "10-2*3",
			expected: 4, // 10-(2*3)
		},
		{
			name:     "деление и умножение",
			expr:     "10/2*5",
			expected: 25, // (10/2)*5
		},
		{
			name:     "сложное выражение",
			expr:     "2+3*4-6/2",
			expected: 11, // 2+(3*4)-(6/2) = 2+12-3 = 11
		},
		{
			name:     "выражение со скобками",
			expr:     "(2+3)*4",
			expected: 20, // (2+3)*4 = 5*4 = 20
		},
		{
			name:     "вложенные скобки",
			expr:     "2*((3+2)*2)",
			expected: 20, // 2*((3+2)*2) = 2*(5*2) = 2*10 = 20
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTest()

			calc := calculator.NewCalculator()
			result, err := calc.Calculate(tt.expr)
			if err != nil {
				t.Fatalf("Ошибка вычисления выражения: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Неверный результат: ожидалось %f, получено %f", tt.expected, result)
			}
		})
	}
}
