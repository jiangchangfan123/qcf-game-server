package llm

import (
	"GameServer/internal/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

// Chat 调用 LLM API（OpenAI 兼容格式）
func Chat(systemPrompt, userPrompt string) (string, error) {
	cfg := config.C.LLM

	reqBody := chatRequest{
		Model: cfg.Model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("POST", cfg.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LLM 返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空结果")
	}

	return chatResp.Choices[0].Message.Content, nil
}
