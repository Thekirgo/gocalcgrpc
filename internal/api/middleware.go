package api

import (
	"context"
	"errors"
	"gocalc/internal/auth"
	"net/http"
)

type ContextKey string

const (
	UserIDKey    ContextKey = "user_id"
	UserLoginKey ContextKey = "user_login"
)

// AuthMiddleware проверяет JWT токен и добавляет данные пользователя в контекст
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := auth.ExtractTokenFromRequest(r)
		if tokenString == "" {
			SendErrorResponse(w, http.StatusUnauthorized, "Unauthorized: no token provided")
			return
		}

		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			status := http.StatusUnauthorized
			message := "Unauthorized: invalid token"

			// Проверяем, истек ли срок действия токена
			if errors.Is(err, auth.ErrExpiredToken) {
				message = "Unauthorized: token has expired"
				// Можно вернуть специальный код 401 с информацией об истекшем токене
				// или использовать код 403 Forbidden для обозначения устаревшего токена
				// Общепринятой практикой является использование 401 + специальное сообщение
			}

			SendErrorResponse(w, status, message)
			return
		}

		// Добавляем данные пользователя в контекст
		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UserLoginKey, claims.Login)

		// Вызываем следующий обработчик с обновленным контекстом
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserIDFromContext извлекает ID пользователя из контекста
func GetUserIDFromContext(ctx context.Context) (int, bool) {
	userID, ok := ctx.Value(UserIDKey).(int)
	return userID, ok
}

// GetUserLoginFromContext извлекает логин пользователя из контекста
func GetUserLoginFromContext(ctx context.Context) (string, bool) {
	login, ok := ctx.Value(UserLoginKey).(string)
	return login, ok
}
