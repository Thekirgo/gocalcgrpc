package parser

import (
	"fmt"
	"gocalc/internal/types"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func infixToRPN(tokens []string) []string {
	precedence := map[string]int{
		"+": 1,
		"-": 1,
		"*": 2,
		"/": 2,
	}

	var output []string
	var stack []string

	for _, token := range tokens {
		switch token {
		case "+", "-", "*", "/":
			for len(stack) > 0 && precedence[stack[len(stack)-1]] >= precedence[token] {
				output = append(output, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, token)
		case "(":
			stack = append(stack, token)
		case ")":
			for len(stack) > 0 && stack[len(stack)-1] != "(" {
				output = append(output, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			output = append(output, token)
		}
	}

	for len(stack) > 0 {
		output = append(output, stack[len(stack)-1])
		stack = stack[:len(stack)-1]
	}

	return output
}

// Разбивает выражение на токены
func tokenize(expr string) []string {
	expr = strings.ReplaceAll(expr, " ", "")
	var tokens []string
	var num strings.Builder

	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch c {
		case '+', '-', '*', '/', '(', ')':
			if num.Len() > 0 {
				tokens = append(tokens, num.String())
				num.Reset()
			}
			tokens = append(tokens, string(c))
		default:
			num.WriteByte(c)
		}
	}
	if num.Len() > 0 {
		tokens = append(tokens, num.String())
	}
	return tokens
}

func ParseExpression(expr string) ([]types.Task, error) {
	tokens := tokenize(expr)
	rpn := infixToRPN(tokens)

	var tasks []types.Task
	var stack []string

	for _, token := range rpn {
		switch token {
		case "+", "-", "*", "/":
			if len(stack) < 2 {
				return nil, fmt.Errorf("invalid expression")
			}

			task := types.Task{
				ID:        uuid.New().String(),
				Operation: token,
			}

			arg2ID := stack[len(stack)-1]
			arg1ID := stack[len(stack)-2]
			stack = stack[:len(stack)-2]

			if val, err := strconv.ParseFloat(arg1ID, 64); err == nil {
				task.Arg1 = val
			} else {
				task.Arg1 = 0
				task.DependsOn = append(task.DependsOn, arg1ID)
			}

			if val, err := strconv.ParseFloat(arg2ID, 64); err == nil {
				task.Arg2 = val
			} else {
				task.Arg2 = 0
				task.DependsOn = append(task.DependsOn, arg2ID)
			}

			if token == "*" || token == "/" {
				task.Priority = 2
			} else {
				task.Priority = 1
			}

			tasks = append(tasks, task)
			stack = append(stack, task.ID)
		default:
			stack = append(stack, token)
		}
	}

	return tasks, nil
}
