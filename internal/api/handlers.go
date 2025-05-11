package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"gocalc/internal/auth"
)

// TokenInfoHandler возвращает информацию о времени жизни токена
func TokenInfoHandler(w http.ResponseWriter, r *http.Request) {
	expirationMinutes := auth.GetTokenExpiration()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"expirationMinutes": strconv.Itoa(expirationMinutes),
	})
}
