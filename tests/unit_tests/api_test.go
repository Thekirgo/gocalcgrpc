package unit_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"gocalc/internal/api"
	"gocalc/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Мок для базы данных
type MockDatabase struct{}

func (m *MockDatabase) SaveExpression(expression *models.Expression, userID int) error {
	return nil
}

func (m *MockDatabase) GetExpressions(userID int) ([]models.Expression, error) {
	return []models.Expression{}, nil
}

func TestCalculatorHandler_Calculate(t *testing.T) {
	originalSaveExpression := api.SaveExpressionFunc
	defer func() { api.SaveExpressionFunc = originalSaveExpression }()

	api.SaveExpressionFunc = func(expression *models.Expression, userID int) error {
		return nil
	}

	tests := []struct {
		name           string
		requestBody    interface{}
		wantStatus     int
		wantResult     *float64
		wantErrMessage string
	}{
		{
			name: "корректное выражение",
			requestBody: api.CalculateRequest{
				Expression: "2+2*2",
			},
			wantStatus: http.StatusOK,
			wantResult: func() *float64 { f := 6.0; return &f }(),
		},
		{
			name: "некорректное выражение",
			requestBody: api.CalculateRequest{
				Expression: "2+a",
			},
			wantStatus:     http.StatusUnprocessableEntity,
			wantErrMessage: "Expression is not valid",
		},
		{
			name:           "пустой запрос",
			requestBody:    nil,
			wantStatus:     http.StatusInternalServerError,
			wantErrMessage: "Internal server error",
		},
		{
			name: "пустое выражение",
			requestBody: api.CalculateRequest{
				Expression: "",
			},
			wantStatus:     http.StatusUnprocessableEntity,
			wantErrMessage: "Expression is not valid",
		},
		{
			name: "деление на ноль",
			requestBody: api.CalculateRequest{
				Expression: "1/0",
			},
			wantStatus:     http.StatusUnprocessableEntity,
			wantErrMessage: "Expression is not valid",
		},
	}

	handler := api.NewCalculatorHandler()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error
			if tt.requestBody != nil {
				body, err = json.Marshal(tt.requestBody)
				if err != nil {
					t.Fatal(err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/calculate", bytes.NewReader(body))

			ctx := context.WithValue(req.Context(), api.UserIDKey, 1)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			handler.Calculate(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Calculate() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantResult != nil {
				var response api.SuccessResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if response.Result != *tt.wantResult {
					t.Errorf("Calculate() result = %v, want %v", response.Result, *tt.wantResult)
				}
			}

			if tt.wantErrMessage != "" {
				var response api.ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if response.Error != tt.wantErrMessage {
					t.Errorf("Calculate() error = %v, want %v", response.Error, tt.wantErrMessage)
				}
			}
		})
	}
}

func TestSendErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		message    string
		wantStatus int
	}{
		{
			name:       "Bad Request",
			status:     http.StatusBadRequest,
			message:    "Invalid input",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Not Found",
			status:     http.StatusNotFound,
			message:    "Resource not found",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Internal Server Error",
			status:     http.StatusInternalServerError,
			message:    "Server error",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			api.SendErrorResponse(w, tt.status, tt.message)

			if w.Code != tt.wantStatus {
				t.Errorf("SendErrorResponse() status = %v, want %v", w.Code, tt.wantStatus)
			}

			var response api.ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Could not parse response: %v", err)
			}

			if response.Error != tt.message {
				t.Errorf("SendErrorResponse() message = %v, want %v", response.Error, tt.message)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("SendErrorResponse() Content-Type = %v, want application/json", contentType)
			}
		})
	}
}

func TestSendSuccessResponse(t *testing.T) {
	tests := []struct {
		name   string
		result float64
	}{
		{
			name:   "Zero result",
			result: 0,
		},
		{
			name:   "Positive result",
			result: 42.5,
		},
		{
			name:   "Negative result",
			result: -10.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			api.SendSuccessResponse(w, tt.result)

			if w.Code != http.StatusOK {
				t.Errorf("SendSuccessResponse() status = %v, want %v", w.Code, http.StatusOK)
			}

			var response api.SuccessResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("Could not parse response: %v", err)
			}

			if response.Result != tt.result {
				t.Errorf("SendSuccessResponse() result = %v, want %v", response.Result, tt.result)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("SendSuccessResponse() Content-Type = %v, want application/json", contentType)
			}
		})
	}
}
