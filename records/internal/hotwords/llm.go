package hotwords

import (
	"context"
	"encoding/json"
	"fmt"
	"records/pkg/logger"
	"regexp"
	"strings"

	jsonrepair "github.com/RealAlexandreAI/json-repair"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// ExtractorConfig LLM 调用配置
type ExtractorConfig struct {
	APIKey              string
	BaseURL             string
	ModelName           string
	Temperature         float64
	MaxCompletionTokens int64
	SystemPrompt        string // 热词抽取的 system prompt，空则使用 defaultSystemPrompt
}

// Extractor 热词抽取 LLM 调用
type Extractor struct {
	client *openai.Client
	cfg    ExtractorConfig
	log    logger.Logger
}

// NewExtractor 创建抽取器
func NewExtractor(cfg ExtractorConfig, log logger.Logger) *Extractor {
	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
	)
	return &Extractor{client: &client, cfg: cfg, log: log}
}

func extractFinalContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	idx := strings.Index(strings.ToLower(raw), "</think>")
	if idx >= 0 {
		raw = strings.TrimSpace(raw[idx+8:])
	}
	re := regexp.MustCompile("(?s)^```(?:json)?\\s*([\\s\\S]*?)```\\s*$")
	if match := re.FindStringSubmatch(raw); len(match) > 1 {
		raw = strings.TrimSpace(match[1])
	}
	return strings.TrimSpace(raw)
}

// Extract 从日志文本中抽取四类关键词，返回结构化结果
func (e *Extractor) Extract(ctx context.Context, logsText string) (*ExtractedPayload, error) {
	systemPrompt := e.cfg.SystemPrompt
	userPrompt := "销售日志：\n\n" + logsText

	e.log.Debug("hotwords extract: user prompt", "prompt", userPrompt)

	resp, err := e.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(e.cfg.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(e.cfg.Temperature),
		MaxCompletionTokens: openai.Int(int64(e.cfg.MaxCompletionTokens)),
	})
	if err != nil {
		return nil, fmt.Errorf("hotwords extract completion: %w", err)
	}
	content := extractFinalContent(resp.Choices[0].Message.Content)

	e.log.Debug("hotwords extract: response", "response", content)

	var payload ExtractedPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		repaired, repairErr := jsonrepair.RepairJSON(content)
		if repairErr != nil {
			return nil, fmt.Errorf("parse extracted json: %w", err)
		}
		if err := json.Unmarshal([]byte(repaired), &payload); err != nil {
			return nil, fmt.Errorf("parse extracted json: %w, %v", err, repaired)
		}
	}
	return &payload, nil
}
