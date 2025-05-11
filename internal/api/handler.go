package api

import (
	"encoding/json"
	"gocalc/internal/calculator"
	"gocalc/internal/database"
	"gocalc/internal/models"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var SaveExpressionFunc = database.SaveExpression

type CalculatorHandler struct{}

func NewCalculatorHandler() *CalculatorHandler {
	return &CalculatorHandler{}
}

type CalculateRequest struct {
	Expression string `json:"expression"`
}

func (h *CalculatorHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		SendErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		SendErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if len(body) == 0 {
		SendErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var req CalculateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		SendErrorResponse(w, http.StatusUnprocessableEntity, "Expression is not valid")
		return
	}

	if strings.TrimSpace(req.Expression) == "" {
		SendErrorResponse(w, http.StatusUnprocessableEntity, "Expression is not valid")
		return
	}

	expression := &models.Expression{
		ID:     uuid.New().String(),
		Text:   req.Expression,
		Status: "processing",
	}

	result, err := calculator.Calc(req.Expression)
	if err != nil {
		// ошибка при вычислении выражения должна возвращать 422
		if strings.Contains(err.Error(), "invalid") ||
			strings.Contains(err.Error(), "division by zero") ||
			strings.Contains(err.Error(), "mismatched parentheses") ||
			strings.Contains(err.Error(), "tokenization error") {

			errorMsg := err.Error()
			errorMsg = strings.Replace(errorMsg, "tokenization error: ", "", 1)

			expression.Status = "error"

			// Сохраняем выражение с ошибкой
			_ = SaveExpressionFunc(expression, userID)

			SendErrorResponse(w, http.StatusUnprocessableEntity, "Expression is not valid")
			return
		}

		expression.Status = "error"
		_ = SaveExpressionFunc(expression, userID)
		SendErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	expression.Status = "completed"
	expression.Result = result

	err = SaveExpressionFunc(expression, userID)
	if err != nil {
	}

	SendSuccessResponse(w, result)
}

func (h *CalculatorHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		SendErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	expressions, err := database.GetExpressions(userID)
	if err != nil {
		SendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve history")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models.ExpressionList{Expressions: expressions})
}
