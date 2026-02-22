package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"records/internal/config"
	"records/internal/models"
	"records/pkg/logger"

	jsonrepair "github.com/RealAlexandreAI/json-repair"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// extractFinalContent 从模型输出中提取最终答案，忽略思考过程
// 支持含思考过程的模型（DeepSeek-R1、Qwen 等），输出 <think>...</think>
// ...最终答案 格式时，只取 </think> 后的内容用于解析
func extractFinalContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	// 支持 </think> <think></think> 等常见变体（不区分大小写）
	idx := strings.Index(strings.ToLower(raw), "</think>")
	if idx >= 0 {
		return strings.TrimSpace(raw[idx+8:])
	}
	return raw
}

// Client AI客户端接口
type Client interface {
	IsCustomerFollowRelated(ctx context.Context, userInput string) (bool, error)
	IsUserConfirmation(ctx context.Context, userInput string) (bool, error)
	IsUserNoMoreCustomers(ctx context.Context, userInput string) (bool, error)
	SemanticAnalysis(ctx context.Context, userInput, stage, focusCustomer, expectedField, conversationHistory string) (*models.SemanticAnalysisResult, error)
	GenerateDialogue(ctx context.Context, stage, focusCustomer, expectedField, userInput, historyContext, summary, conversationHistory string) (string, error)
	SummarizeCustomerInfo(ctx context.Context, customerFollowRecords string) (string, error)
	EntityNormalization(ctx context.Context, request *models.NormalizationRequest) ([]models.NormalizationResult, error)
}

// OpenAIClient OpenAI客户端实现
type OpenAIClient struct {
	client   *openai.Client
	config   config.AI
	messages config.Messages
	prompts  config.Prompts
	logger   logger.Logger
}

// NewOpenAIClient 创建OpenAI客户端
func NewOpenAIClient(cfg config.AI, prompts config.Prompts, logger logger.Logger) *OpenAIClient {
	client := openai.NewClient(
		option.WithAPIKey(cfg.OpenAI.APIKey),
		option.WithBaseURL(cfg.OpenAI.BaseURL),
	)

	return &OpenAIClient{
		client:  &client,
		config:  cfg,
		prompts: prompts,
		logger:  logger,
	}
}

// IsCustomerFollowRelated 判断对话是否和客户跟进相关
func (c *OpenAIClient) IsCustomerFollowRelated(ctx context.Context, userInput string) (bool, error) {
	systemPrompt := c.prompts.IsCustomerFollowRelated
	userPrompt := fmt.Sprintf(`用户输入：%s`, userInput)
	response, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.config.Semantic.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(c.config.Semantic.Temperature),
		MaxCompletionTokens: openai.Int(c.config.Semantic.MaxCompletionTokens),
	})

	if err != nil {
		c.logger.Error("Is customer follow related failed", "error", err)
		return false, fmt.Errorf("is customer follow related failed: %w", err)
	}

	return extractFinalContent(response.Choices[0].Message.Content) == "true", nil
}

// IsUserConfirmation 判断用户是否给出肯定确认（CONFIRMING 阶段进入 OUTPUTTING）
func (c *OpenAIClient) IsUserConfirmation(ctx context.Context, userInput string) (bool, error) {
	if c.prompts.IsUserConfirmation == "" {
		return false, nil
	}
	systemPrompt := c.prompts.IsUserConfirmation
	userPrompt := fmt.Sprintf(`用户输入：%s`, userInput)
	response, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.config.Semantic.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(c.config.Semantic.Temperature),
		MaxCompletionTokens: openai.Int(c.config.Semantic.MaxCompletionTokens),
	})
	if err != nil {
		c.logger.Error("Is user confirmation failed", "error", err)
		return false, fmt.Errorf("is user confirmation failed: %w", err)
	}
	return extractFinalContent(response.Choices[0].Message.Content) == "true", nil
}

// IsUserNoMoreCustomers 判断用户是否表示没有其他客户
func (c *OpenAIClient) IsUserNoMoreCustomers(ctx context.Context, userInput string) (bool, error) {
	if c.prompts.IsUserNoMoreCustomers == "" {
		return false, nil
	}
	systemPrompt := c.prompts.IsUserNoMoreCustomers
	userPrompt := fmt.Sprintf(`用户输入：%s`, userInput)
	response, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.config.Semantic.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(c.config.Semantic.Temperature),
		MaxCompletionTokens: openai.Int(c.config.Semantic.MaxCompletionTokens),
	})
	if err != nil {
		c.logger.Error("IsUserNoMoreCustomers failed", "error", err)
		return false, fmt.Errorf("is user no more customers failed: %w", err)
	}
	return extractFinalContent(response.Choices[0].Message.Content) == "true", nil
}

// SemanticAnalysis 语义分析
func (c *OpenAIClient) SemanticAnalysis(ctx context.Context, userInput, stage, focusCustomer, expectedField, conversationHistory string) (*models.SemanticAnalysisResult, error) {
	systemPrompt := c.prompts.SemanticAnalysis

	conversationPrefix := ""
	if conversationHistory != "" {
		conversationPrefix = fmt.Sprintf("此前对话内容：\n%s\n\n", conversationHistory)
	}

	// 构建用户提示，CONFIRMING 阶段显式强调将修正归入当前关注客户
	extraHint := ""
	if stage == models.StatusConfirming && focusCustomer != "" {
		extraHint = fmt.Sprintf("（重要：用户若对复述内容提出修正，必须将修正字段归入【%s】的 field_updates，即使用户未重复客户名）\n", focusCustomer)
	}

	userPrompt := fmt.Sprintf(`当前会话阶段：%s
当前关注的客户：%s
当前客户所需信息点：%s
%s%s用户输入：%s`, stage, focusCustomer, expectedField, extraHint, conversationPrefix, userInput)

	c.logger.Debug("Semantic analysis user prompt", "userPrompt", userPrompt)

	response, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.config.Semantic.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(c.config.Semantic.Temperature),
		MaxCompletionTokens: openai.Int(c.config.Semantic.MaxCompletionTokens),
	})

	if err != nil {
		c.logger.Error("Semantic analysis failed", "error", err)
		return nil, fmt.Errorf("semantic analysis failed: %w", err)
	}

	content := extractFinalContent(response.Choices[0].Message.Content)

	c.logger.Debug("Semantic analysis result:", "content", content)

	var result models.SemanticAnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		c.logger.Error("Failed to parse semantic analysis result", "error", err, "content", content)
		// 修复 JSON
		repairedContent, err := jsonrepair.RepairJSON(content)
		if err != nil {
			c.logger.Error("Failed to repair semantic analysis result", "error", err, "content", content)
			return nil, fmt.Errorf("failed to repair semantic analysis result: %w", err)
		}
		if err := json.Unmarshal([]byte(repairedContent), &result); err != nil {
			c.logger.Error("Failed to parse semantic analysis result", "error", err, "content", content)
			return nil, fmt.Errorf("failed to parse semantic analysis result: %w", err)
		}
	}

	return &result, nil
}

// GenerateDialogue 生成对话
func (c *OpenAIClient) GenerateDialogue(ctx context.Context, stage, focusCustomer, expectedField, userInput, historyContext, summary, conversationHistory string) (string, error) {
	var systemPrompt string
	var userPrompt string

	switch stage {
	case models.StatusCollecting:
		systemPrompt = c.prompts.DialogueCollecting

		conversationPrefix := ""
		if conversationHistory != "" {
			conversationPrefix = fmt.Sprintf("此前对话内容：\n%s\n\n", conversationHistory)
		}
		userPrompt = fmt.Sprintf(`请生成下一句你要对用户说的话。
当前对话背景：
- 这是一次工作跟进的复盘对话
- 允许信息不完整、顺序混乱
- 重点是复盘发生了什么，而不是填写信息

当前聚焦客户：
%s

希望收集的信息：
%s

已知跟进情况摘要：
%s

%s用户刚刚说：
%s

请你自然地继续这段对话。`, focusCustomer, expectedField, summary, conversationPrefix, userInput)

	case models.StatusAskingOtherCustomers:
		// 固定文案，无需调用模型（generateReply 会直接返回，此处为兜底）
		return c.messages.AskingOtherCustomers, nil

	case models.StatusConfirming:
		systemPrompt = c.prompts.DialogueConfirming

		userPrompt = fmt.Sprintf(`根据以下跟进记录和用户说的，生成下一句你要对用户说的话。
注意：跟进记录（JSON）即为待确认的全部数据，请仅基于此复述，不得虚构、杜撰或补充任何未出现的信息。

跟进记录（JSON）：%s
用户刚才说：%s`, historyContext, userInput)

	default:
		return "", fmt.Errorf("unsupported stage: %s", stage)
	}

	c.logger.Debug("Dialogue generating user prompt", "user_prompt", userPrompt)

	response, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.config.Dialogue.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(c.config.Dialogue.Temperature),
		MaxCompletionTokens: openai.Int(c.config.Dialogue.MaxCompletionTokens),
	})

	if err != nil {
		c.logger.Error("Dialogue generation failed", "error", err)
		return "", fmt.Errorf("dialogue generation failed: %w", err)
	}

	c.logger.Debug("Dialogue generating result", "content", response.Choices[0].Message.Content)

	return extractFinalContent(response.Choices[0].Message.Content), nil
}

// SummarizeCustomerInfo 总结客户跟进信息
func (c *OpenAIClient) SummarizeCustomerInfo(ctx context.Context, customerFollowRecords string) (string, error) {
	systemPrompt := c.prompts.CustomerSummary

	userPrompt := fmt.Sprintf(`以下是某一客户已经确认过的跟进事实，请将其整理为可用于对话中的自然复盘摘要。
输入事实（JSON）：%s`, customerFollowRecords)

	c.logger.Debug("Customer info summarization user prompt", "userPrompt", userPrompt)

	response, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.config.Dialogue.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(c.config.Dialogue.Temperature),
		MaxCompletionTokens: openai.Int(c.config.Dialogue.MaxCompletionTokens),
	})

	if err != nil {
		c.logger.Error("Customer info summarization failed", "error", err)
		return "", fmt.Errorf("customer info summarization failed: %w", err)
	}

	c.logger.Debug("Customer info summarization result", "content", response.Choices[0].Message.Content)

	return extractFinalContent(response.Choices[0].Message.Content), nil
}

// EntityNormalization 客户/联系人归一评分
func (c *OpenAIClient) EntityNormalization(ctx context.Context, request *models.NormalizationRequest) ([]models.NormalizationResult, error) {
	systemPrompt := c.prompts.EntityNormalization

	// 构建用户提示词
	dialogContextJSON, _ := json.Marshal(request.DialogContext)
	mentionsJSON, _ := json.Marshal(request.MentionsEntity)
	candidatesJSON, _ := json.Marshal(request.CandidateEntities)

	userPrompt := fmt.Sprintf(`对话上下文：
%s

已抽取的客户/联系人实体：
%s

候选客户/联系人实体：
%s`, string(dialogContextJSON), string(mentionsJSON), string(candidatesJSON))

	c.logger.Debug("Entity normalization user prompt", "userPrompt", userPrompt)

	response, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(c.config.Semantic.ModelName),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature:         openai.Float(c.config.Semantic.Temperature),
		MaxCompletionTokens: openai.Int(c.config.Semantic.MaxCompletionTokens),
	})

	if err != nil {
		c.logger.Error("Entity normalization failed", "error", err)
		return nil, fmt.Errorf("entity normalization failed: %w", err)
	}

	content := extractFinalContent(response.Choices[0].Message.Content)

	c.logger.Debug("Entity normalization result", "content", content)

	var results []models.NormalizationResult
	if err := json.Unmarshal([]byte(content), &results); err != nil {
		c.logger.Error("Failed to parse entity normalization result", "error", err, "content", content)
		return nil, fmt.Errorf("failed to parse entity normalization result: %w", err)
	}

	// 根据评分设置归一等级和是否需要确认
	for i := range results {
		score := results[i].NormalizationScore
		if score >= 80 {
			results[i].NormalizationLevel = "high"
			results[i].NeedsConfirmation = false
		} else if score >= 60 {
			results[i].NormalizationLevel = "medium"
			results[i].NeedsConfirmation = true
		} else if score >= 40 {
			results[i].NormalizationLevel = "low"
			results[i].NeedsConfirmation = true
		} else {
			results[i].NormalizationLevel = "none"
			results[i].NeedsConfirmation = false
		}
	}

	return results, nil
}
