package database

import (
	"database/sql"
	"fmt"
	"gocalc/internal/models"
	"log"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"golang.org/x/crypto/bcrypt"
)

var (
	db   *sql.DB
	once sync.Once
)

// GetDB возвращает экземпляр соединения с базой данных
func GetDB() *sql.DB {
	once.Do(func() {
		var err error
		db, err = sql.Open("sqlite", "./calculator.db")
		if err != nil {
			panic(fmt.Sprintf("Не удалось подключиться к базе данных: %v", err))
		}

		createTables()
	})

	return db
}

func createTables() {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			login TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL
		)
	`)
	if err != nil {
		panic(fmt.Sprintf("Ошибка создания таблицы users: %v", err))
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS expressions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			text TEXT NOT NULL,
			status TEXT NOT NULL,
			result REAL,
			created_at TEXT NOT NULL DEFAULT (strftime('%d.%m.%Y %H:%M:%S', 'now')),
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		panic(fmt.Sprintf("Ошибка создания таблицы expressions: %v", err))
	}

	applyMigrations()
}

// applyMigrations применяет все миграции к базе данных
func applyMigrations() {
	// Проверяем существование столбца created_at
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('expressions') WHERE name='created_at'").Scan(&count)
	if err != nil {
		panic(fmt.Sprintf("Ошибка проверки существования столбца created_at: %v", err))
	}

	// Если столбец не существует, добавляем его
	if count == 0 {
		_, err = db.Exec(`
			ALTER TABLE expressions 
			ADD COLUMN created_at TEXT NOT NULL DEFAULT (strftime('%d.%m.%Y %H:%M:%S', 'now'))
		`)
		if err != nil {
			panic(fmt.Sprintf("Ошибка добавления столбца created_at: %v", err))
		}
	}
}

// CreateUser создает нового пользователя в базе данных
func CreateUser(login, password string) (int, error) {
	// Проверяем, существует ли пользователь с таким логином
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE login = ?", login).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка проверки существования пользователя: %w", err)
	}

	if count > 0 {
		return 0, fmt.Errorf("пользователь с логином %s уже существует", login)
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("ошибка хеширования пароля: %w", err)
	}

	// Вставляем нового пользователя
	result, err := db.Exec("INSERT INTO users (login, password) VALUES (?, ?)", login, string(hashedPassword))
	if err != nil {
		return 0, fmt.Errorf("ошибка создания пользователя: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("ошибка получения ID пользователя: %w", err)
	}

	return int(id), nil
}

func GetUser(login string) (*models.User, error) {
	var user models.User
	err := db.QueryRow("SELECT id, login, password FROM users WHERE login = ?", login).Scan(&user.ID, &user.Login, &user.Password)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Пользователь не найден
		}
		return nil, fmt.Errorf("ошибка получения пользователя: %w", err)
	}

	return &user, nil
}

// SaveExpression сохраняет выражение в базе данных
func SaveExpression(expression *models.Expression, userID int) error {
	// Если дата не установлена, устанавливаем текущую
	if expression.CreatedAt == "" {
		expression.CreatedAt = time.Now().Format("02.01.2006 15:04:05")
	}

	_, err := db.Exec(
		"INSERT INTO expressions (id, user_id, text, status, result, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		expression.ID, userID, expression.Text, expression.Status, expression.Result, expression.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("ошибка сохранения выражения: %w", err)
	}

	log.Printf("СОХРАНЕНО В БД: ID=%s, userID=%d, text='%s', status=%s, result=%f, created_at=%s",
		expression.ID, userID, expression.Text, expression.Status, expression.Result, expression.CreatedAt)

	return nil
}

// GetExpressions возвращает все выражения пользователя
func GetExpressions(userID int) ([]models.Expression, error) {
	rows, err := db.Query("SELECT id, text, status, result, created_at FROM expressions WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения выражений: %w", err)
	}
	defer rows.Close()

	var expressions []models.Expression
	for rows.Next() {
		var expr models.Expression
		err := rows.Scan(&expr.ID, &expr.Text, &expr.Status, &expr.Result, &expr.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения данных выражения: %w", err)
		}
		expressions = append(expressions, expr)
	}

	return expressions, nil
}

// CheckPasswordHash сравнивает пароль и хеш пароля
func CheckPasswordHash(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
