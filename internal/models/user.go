package models

// User представляет модель пользователя в системе
type User struct {
	ID       int    `json:"id"`
	Login    string `json:"login"`
	Password string `json:"-"` // Не сериализуем пароль в JSON
}

// LoginRequest представляет запрос на вход в систему
type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// RegisterRequest представляет запрос на регистрацию
type RegisterRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// AuthResponse представляет ответ после успешной аутентификации
type AuthResponse struct {
	Token string `json:"token"`
}
