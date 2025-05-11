package unit_tests

import (
	"encoding/json"
	"gocalc/internal/types"
	"testing"
)

func TestExpressionSerialization(t *testing.T) {
	tests := []struct {
		name     string
		expr     types.Expression
		expected string
	}{
		{
			name: "положительный результат",
			expr: types.Expression{
				ID:        "test-1",
				Original:  "2+3",
				Status:    "COMPLETED",
				Result:    5.0,
				CreatedAt: "01.01.2024 12:00:00",
			},
			expected: `{"id":"test-1","expression":"2+3","status":"COMPLETED","result":5,"created_at":"01.01.2024 12:00:00"}`,
		},
		{
			name: "отрицательный результат",
			expr: types.Expression{
				ID:        "test-2",
				Original:  "2-5",
				Status:    "COMPLETED",
				Result:    -3.0,
				CreatedAt: "01.01.2024 12:00:00",
			},
			expected: `{"id":"test-2","expression":"2-5","status":"COMPLETED","result":-3,"created_at":"01.01.2024 12:00:00"}`,
		},
		{
			name: "нулевой результат",
			expr: types.Expression{
				ID:        "test-3",
				Original:  "2-2",
				Status:    "COMPLETED",
				Result:    0.0,
				CreatedAt: "01.01.2024 12:00:00",
			},
			expected: `{"id":"test-3","expression":"2-2","status":"COMPLETED","result":0,"created_at":"01.01.2024 12:00:00"}`,
		},
		{
			name: "в процессе обработки",
			expr: types.Expression{
				ID:        "test-4",
				Original:  "2+2",
				Status:    "PROCESSING",
				CreatedAt: "01.01.2024 12:00:00",
			},
			expected: `{"id":"test-4","expression":"2+2","status":"PROCESSING","result":0,"created_at":"01.01.2024 12:00:00"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.expr)
			if err != nil {
				t.Fatalf("Ошибка маршалинга: %v", err)
			}

			if string(data) != tt.expected {
				t.Errorf("Неверный результат маршалинга: получено %s, ожидалось %s", string(data), tt.expected)
			}
		})
	}
}

func TestOrchestratorExpressionSerialization(t *testing.T) {
	expr := types.Expression{
		ID:       "test-id",
		Original: "2-2",
		Status:   "COMPLETED",
		Result:   0.0,
	}

	data, err := json.Marshal(expr)
	if err != nil {
		t.Fatalf("Ошибка маршалинга: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Ошибка анмаршалинга: %v", err)
	}

	if _, exists := result["result"]; !exists {
		t.Errorf("Поле 'result' отсутствует в JSON: %s", string(data))
	}

	var decodedExpr types.Expression
	if err := json.Unmarshal(data, &decodedExpr); err != nil {
		t.Fatalf("Ошибка декодирования: %v", err)
	}

	if decodedExpr.Result != 0.0 {
		t.Errorf("Неверное значение результата: получено %f, ожидалось 0.0", decodedExpr.Result)
	}
}
