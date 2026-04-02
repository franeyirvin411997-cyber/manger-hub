package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"multi_node_platform/pkg/models"
)

type Agent struct {
	baseURL string
	apiKey  string
	model   string
}

type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type Tool struct {
	Type     string         `json:"type"`
	Function ToolDefinition `json:"function"`
}

type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type ChatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

type ChatResult struct {
	Response   string `json:"response"`
	ToolsUsed  []string `json:"tools_used"`
}

func NewAgent() *Agent {
	return &Agent{
		baseURL: models.GetConfig("ai_base_url"),
		apiKey:  models.GetConfig("ai_api_key"),
		model:   models.GetConfig("ai_model"),
	}
}

const systemPrompt = `你是多节点流量运营平台的 AI 运维助手。你可以：
1. 查询系统状态（节点、代理、代理组、账号、浏览器）
2. 执行运维操作（部署、停止、重启、迁移、换代理）
3. 管理账号（查看状态）
4. 控制浏览器（截图、导航）

当用户描述需求时，使用对应的工具来完成。如果操作有风险（如删除、停止），先说明影响再执行。
回答使用中文。简洁明了。`

func (a *Agent) Chat(userMessage string, source string) (*ChatResult, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("AI API Key 未配置，请在系统配置中设置")
	}

	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	toolsUsed := []string{}
	maxRounds := 5

	for round := 0; round < maxRounds; round++ {
		resp, err := a.callLLM(messages)
		if err != nil {
			return nil, err
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("LLM 返回空响应")
		}

		choice := resp.Choices[0]
		messages = append(messages, choice.Message)

		if choice.FinishReason == "tool_calls" || len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				toolsUsed = append(toolsUsed, tc.Function.Name)
				result := ExecuteTool(tc.Function.Name, tc.Function.Arguments)
				messages = append(messages, Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		// 最终文本响应
		actionLog := models.AIActionLog{
			ID:          uuid.New().String(),
			Source:      source,
			UserInput:   userMessage,
			ParsedTools: toJSON(toolsUsed),
			Result:      choice.Message.Content,
			Status:      "success",
			CreatedAt:   time.Now(),
		}
		models.DB.Create(&actionLog)

		return &ChatResult{
			Response:  choice.Message.Content,
			ToolsUsed: toolsUsed,
		}, nil
	}

	return nil, fmt.Errorf("AI 工具调用轮次超限")
}

func (a *Agent) callLLM(messages []Message) (*ChatResponse, error) {
	reqBody := ChatRequest{
		Model:    a.model,
		Messages: messages,
		Tools:    GetToolDefinitions(),
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", a.baseURL+"/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LLM API 调用失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		log.Printf("[AI] LLM API 错误 %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("LLM API 返回 %d", resp.StatusCode)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %v", err)
	}
	return &chatResp, nil
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
