package unit_tests

import (
	"gocalc/internal/types"
	"testing"
	"time"
)

func calculateResultTest(task types.Task) float64 {
	time.Sleep(10 * time.Millisecond)

	switch task.Operation {
	case "+":
		return task.Arg1 + task.Arg2
	case "-":
		return task.Arg1 - task.Arg2
	case "*":
		return task.Arg1 * task.Arg2
	case "/":
		if task.Arg2 == 0 {
			return 0
		}
		return task.Arg1 / task.Arg2
	default:
		return 0
	}
}

func TestCalculateResult(t *testing.T) {
	tests := []struct {
		name string
		task types.Task
		want float64
	}{
		{
			name: "Сложение",
			task: types.Task{
				ID:        "test-1",
				Arg1:      2,
				Arg2:      3,
				Operation: "+",
			},
			want: 5,
		},
		{
			name: "Вычитание",
			task: types.Task{
				ID:        "test-2",
				Arg1:      5,
				Arg2:      3,
				Operation: "-",
			},
			want: 2,
		},
		{
			name: "Умножение",
			task: types.Task{
				ID:        "test-3",
				Arg1:      2,
				Arg2:      3,
				Operation: "*",
			},
			want: 6,
		},
		{
			name: "Деление",
			task: types.Task{
				ID:        "test-4",
				Arg1:      6,
				Arg2:      3,
				Operation: "/",
			},
			want: 2,
		},
		{
			name: "Деление на ноль",
			task: types.Task{
				ID:        "test-5",
				Arg1:      6,
				Arg2:      0,
				Operation: "/",
			},
			want: 0,
		},
		{
			name: "Неизвестная операция",
			task: types.Task{
				ID:        "test-6",
				Arg1:      6,
				Arg2:      3,
				Operation: "?",
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateResultTest(tt.task)
			if got != tt.want {
				t.Errorf("calculateResult() = %v, want %v", got, tt.want)
			}
		})
	}
}
