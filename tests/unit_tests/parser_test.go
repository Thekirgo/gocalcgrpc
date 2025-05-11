package unit_tests

import (
	"gocalc/internal/calculator"
	"testing"
)

func TestRPN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "простое сложение",
			input:    "2+2",
			expected: "2 2 +",
		},
		{
			name:     "сложное выражение",
			input:    "(2+2)*2",
			expected: "2 2 + 2 *",
		},
		{
			name:     "выражение с приоритетами",
			input:    "0 + 6 - 1 + (2*100)",
			expected: "0 6 + 1 - 2 100 * +",
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
