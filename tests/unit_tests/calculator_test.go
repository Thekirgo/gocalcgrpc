package unit_tests

import (
	"gocalc/internal/calculator"
	"strings"
	"testing"
)

func TestCalc(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "простое сложение",
			input:   "2+2",
			want:    4,
			wantErr: false,
		},
		{
			name:    "сложное выражение",
			input:   "(2+2)*2",
			want:    8,
			wantErr: false,
		},
		{
			name:    "деление",
			input:   "4/2",
			want:    2,
			wantErr: false,
		},
		{
			name:    "некорректный символ",
			input:   "2+a",
			want:    0,
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "деление на ноль",
			input:   "2/0",
			want:    0,
			wantErr: true,
			errMsg:  "division by zero",
		},
		{
			name:    "несоответствие скобок",
			input:   "(2+2",
			want:    0,
			wantErr: true,
			errMsg:  "mismatched parentheses",
		},

		{
			name:    "умножение и сложение",
			input:   "2+3*4",
			want:    14, // 2+(3*4)
			wantErr: false,
		},
		{
			name:    "умножение и вычитание",
			input:   "10-5*2",
			want:    0, // 10-(5*2)
			wantErr: false,
		},
		{
			name:    "деление и сложение",
			input:   "8/4+3",
			want:    5, // (8/4)+3
			wantErr: false,
		},
		{
			name:    "сложная операция с приоритетами",
			input:   "2+3*4-6/2",
			want:    11, // 2+(3*4)-(6/2)
			wantErr: false,
		},
		{
			name:    "несколько умножений и делений",
			input:   "2*3*4/2",
			want:    12, // ((2*3)*4)/2
			wantErr: false,
		},
		{
			name:    "скобки изменяют приоритет",
			input:   "(2+3)*4",
			want:    20, // (2+3)*4
			wantErr: false,
		},
		{
			name:    "вложенные скобки",
			input:   "2*((3+2)*2)",
			want:    20, // 2*((3+2)*2)
			wantErr: false,
		},
		{
			name:    "сложение, вычитание и умножение",
			input:   "5-2*3+1",
			want:    0, // 5-(2*3)+1
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := calculator.NewCalculator()
			got, err := calc.Calculate(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Calc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Calc() error = %v, wantErr %v", err, tt.errMsg)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("Calc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRPNPriority(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "сложение и умножение",
			input:    "2+3*4",
			expected: "2 3 4 * +",
		},
		{
			name:     "умножение и вычитание",
			input:    "10-5*2",
			expected: "10 5 2 * -",
		},
		{
			name:     "деление и сложение",
			input:    "8/4+3",
			expected: "8 4 / 3 +",
		},
		{
			name:     "сложная операция",
			input:    "2+3*4-6/2",
			expected: "2 3 4 * + 6 2 / -",
		},
		{
			name:     "несколько умножений",
			input:    "2*3*4",
			expected: "2 3 * 4 *",
		},
		{
			name:     "скобки изменяют приоритет",
			input:    "(2+3)*4",
			expected: "2 3 + 4 *",
		},
		{
			name:     "вложенные скобки",
			input:    "2*((3+2)*2)",
			expected: "2 3 2 + 2 * *",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := calculator.NewCalculator()
			err := calc.Tokenize(tt.input)
			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			rpn, err := calc.ToRPN()
			if err != nil {
				t.Fatalf("ToRPN() error = %v", err)
			}

			var rpnStr string
			for i, token := range rpn {
				if i > 0 {
					rpnStr += " "
				}
				rpnStr += token.Value
			}

			if rpnStr != tt.expected {
				t.Errorf("ToRPN() = %v, want %v", rpnStr, tt.expected)
			}
		})
	}
}
