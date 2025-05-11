package types

type Task struct {
	ID            string   `json:"id"`
	Arg1          float64  `json:"arg1"`
	Arg2          float64  `json:"arg2"`
	Operation     string   `json:"operation"`
	OperationTime int      `json:"operation_time"`
	Priority      int      `json:"priority"`
	DependsOn     []string `json:"depends_on,omitempty"`
}

type TaskResult struct {
	ID     string  `json:"id"`
	Result float64 `json:"result"`
}

type Expression struct {
	ID        string  `json:"id"`
	Original  string  `json:"expression"`
	Status    string  `json:"status"`
	Result    float64 `json:"result"`
	CreatedAt string  `json:"created_at"`
}

type CalculateRequest struct {
	Expression string `json:"expression"`
}

type ExpressionResponse struct {
	Expressions []Expression `json:"expressions"`
}
