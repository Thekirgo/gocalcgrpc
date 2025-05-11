package calculator

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type TokenType string

const (
	Number     TokenType = "number"
	Operator   TokenType = "operator"
	LeftParen  TokenType = "left_paren"
	RightParen TokenType = "right_paren"
)

type Token struct {
	Type  TokenType
	Value string
}

type Calculator struct {
	tokens []Token
}

func NewCalculator() *Calculator {
	return &Calculator{}
}

func Calc(expr string) (float64, error) {
	calc := NewCalculator()
	return calc.Calculate(expr)
}

func (c *Calculator) Calculate(expr string) (float64, error) {
	if err := c.Tokenize(expr); err != nil {
		return 0, fmt.Errorf("tokenization error: %v", err)
	}

	rpn, err := c.ToRPN()
	if err != nil {
		return 0, fmt.Errorf("RPN conversion error: %v", err)
	}

	return c.EvaluateRPN(rpn)
}

func (c *Calculator) Tokenize(expr string) error {
	if expr == "" {
		return errors.New("empty expression")
	}

	c.tokens = []Token{}
	expr = strings.ReplaceAll(expr, " ", "")

	for i := 0; i < len(expr); i++ {
		char := expr[i]

		switch {
		case char == '(':
			c.tokens = append(c.tokens, Token{Type: LeftParen, Value: "("})
		case char == ')':
			c.tokens = append(c.tokens, Token{Type: RightParen, Value: ")"})
		case char == '+' || char == '-' || char == '*' || char == '/':
			c.tokens = append(c.tokens, Token{Type: Operator, Value: string(char)})
		case unicode.IsDigit(rune(char)):
			j := i
			for j < len(expr) && (unicode.IsDigit(rune(expr[j])) || expr[j] == '.') {
				j++
			}
			c.tokens = append(c.tokens, Token{Type: Number, Value: expr[i:j]})
			i = j - 1
		default:
			return fmt.Errorf("invalid character: %c", char)
		}
	}

	return nil
}

func (c *Calculator) ToRPN() ([]Token, error) {
	var output []Token
	var stack []Token

	precedence := map[string]int{
		"+": 1,
		"-": 1,
		"*": 2,
		"/": 2,
	}

	for _, token := range c.tokens {
		switch token.Type {
		case Number:
			output = append(output, token)
		case Operator:
			for len(stack) > 0 && stack[len(stack)-1].Type == Operator &&
				precedence[stack[len(stack)-1].Value] >= precedence[token.Value] {
				output = append(output, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, token)
		case LeftParen:
			stack = append(stack, token)
		case RightParen:
			foundLeftParen := false
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.Type == LeftParen {
					foundLeftParen = true
					break
				}
				output = append(output, top)
			}
			if !foundLeftParen {
				return nil, errors.New("mismatched parentheses")
			}
		}
	}

	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if top.Type == LeftParen {
			return nil, errors.New("mismatched parentheses")
		}
		output = append(output, top)
	}

	return output, nil
}

func (c *Calculator) EvaluateRPN(rpn []Token) (float64, error) {
	var stack []float64

	for _, token := range rpn {
		switch token.Type {
		case Number:
			num, err := strconv.ParseFloat(token.Value, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number: %s", token.Value)
			}
			stack = append(stack, num)
		case Operator:
			if len(stack) < 2 {
				return 0, errors.New("invalid expression")
			}

			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]

			var result float64
			switch token.Value {
			case "+":
				result = a + b
			case "-":
				result = a - b
			case "*":
				result = a * b
			case "/":
				if b == 0 {
					return 0, errors.New("division by zero")
				}
				result = a / b
			}

			stack = append(stack, result)
		}
	}

	if len(stack) != 1 {
		return 0, errors.New("invalid expression")
	}

	return stack[0], nil
}
