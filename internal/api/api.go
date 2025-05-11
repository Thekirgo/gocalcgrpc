package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"gocalc/internal/auth"
	"gocalc/internal/calculator"
)

// SetupRouter настраивает маршруты для API
func SetupRouter(calc *calculator.Calculator) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Публичные маршруты (без аутентификации)
	r.Group(func(r chi.Router) {
		r.Post("/register", RegisterHandler)
		r.Post("/login", LoginHandler)
		r.Get("/token-info", TokenInfoHandler)
	})

	// Защищенные маршруты (с аутентификацией)
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Post("/calculate", func(w http.ResponseWriter, r *http.Request) {
			CalculateHandler(w, r, calc)
		})
		r.Get("/history", HistoryHandler)
	})

	return r
}

// Функции-заглушки (ими должны быть фактические обработчики)
func RegisterHandler(w http.ResponseWriter, r *http.Request)                               {}
func LoginHandler(w http.ResponseWriter, r *http.Request)                                  {}
func CalculateHandler(w http.ResponseWriter, r *http.Request, calc *calculator.Calculator) {}
func HistoryHandler(w http.ResponseWriter, r *http.Request)                                {}
