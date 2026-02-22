package main

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type OpenAIParams struct {
	ModeName            string  `json:"mode_name"`
	APIKey              string  `json:"api_key"`
	BaseURL             string  `json:"base_url"`
	FrequencyPenalty    float64 `json:"frequency_penalty"`
	TopP                float64 `json:"top_p"`
	MaxCompletionTokens int64   `json:"max_completion_tokens"`
	Temperature         float64 `json:"temperature"`
}

func openaiReply(params *OpenAIParams, systemPrompt string, userPrompt string) (*string, error) {
	client := openai.NewClient(option.WithAPIKey(params.APIKey), option.WithBaseURL(params.BaseURL))
	ctx := context.Background()

	chatParams := openai.ChatCompletionNewParams{
		Model: params.ModeName,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		PromptCacheKey:      openai.String("unique-prompt-key"),
		SafetyIdentifier:    openai.String("hashed-user-identifier"),
		FrequencyPenalty:    openai.Float(params.FrequencyPenalty),
		TopP:                openai.Float(params.TopP),
		MaxCompletionTokens: openai.Int(params.MaxCompletionTokens),
		Temperature:         openai.Float(params.Temperature),
	}

	response, err := client.Chat.Completions.New(ctx, chatParams)
	if err != nil {
		return nil, err
	}

	return &response.Choices[0].Message.Content, nil
}
