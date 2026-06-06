package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOpenAIBaseURL     = "https://api.openai.com/v1"
	defaultOpenAIModel       = "gpt-4o-mini"
	defaultOpenAITimeout     = 60 * time.Second
	defaultOpenAIMaxRetries  = 2
	defaultOpenAIBackoffBase = 250 * time.Millisecond
)

type openAIChatClient struct {
	baseURL        string
	apiKey         string
	organization   string
	project        string
	model          string
	http           *http.Client
	temperature    *float64
	maxTokens      *int
	seed           *int
	maxRetries     int
	backoffBase    time.Duration
	responseFormat bool
}

type chatCompletionRequest struct {
	Model          string              `json:"model"`
	Messages       []chatMessage       `json:"messages"`
	Temperature    *float64            `json:"temperature,omitempty"`
	MaxTokens      *int                `json:"max_tokens,omitempty"`
	Seed           *int                `json:"seed,omitempty"`
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type,omitempty"`
		Code    any    `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

func openAIClientFromEnv() (*openAIChatClient, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is required for examples/agent")
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("OPENAI_BASE_URL: %w", err)
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = defaultOpenAIModel
	}

	temperature, err := optionalFloat("OPENAI_TEMPERATURE")
	if err != nil {
		return nil, err
	}
	maxTokens, err := optionalInt("OPENAI_MAX_TOKENS")
	if err != nil {
		return nil, err
	}
	seed, err := optionalInt("OPENAI_SEED")
	if err != nil {
		return nil, err
	}
	timeout, err := optionalDurationSeconds("OPENAI_TIMEOUT_SECONDS", defaultOpenAITimeout)
	if err != nil {
		return nil, err
	}
	maxRetries, err := optionalIntValue("OPENAI_MAX_RETRIES", defaultOpenAIMaxRetries)
	if err != nil {
		return nil, err
	}
	responseFormat, err := optionalBoolValue("OPENAI_RESPONSE_FORMAT", true)
	if err != nil {
		return nil, err
	}

	return &openAIChatClient{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		organization:   strings.TrimSpace(os.Getenv("OPENAI_ORG_ID")),
		project:        strings.TrimSpace(os.Getenv("OPENAI_PROJECT_ID")),
		model:          model,
		http:           &http.Client{Timeout: timeout},
		temperature:    temperature,
		maxTokens:      maxTokens,
		seed:           seed,
		maxRetries:     maxRetries,
		backoffBase:    defaultOpenAIBackoffBase,
		responseFormat: responseFormat,
	}, nil
}

func (c *openAIChatClient) Complete(ctx context.Context, args llmArgs) (llmResult, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		result, retry, err := c.completeOnce(ctx, args)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retry || attempt == c.maxRetries {
			break
		}
		if err := sleepContext(ctx, c.backoff(attempt)); err != nil {
			return llmResult{}, err
		}
	}
	return llmResult{}, lastErr
}

func (c *openAIChatClient) completeOnce(ctx context.Context, args llmArgs) (llmResult, bool, error) {
	body, err := json.Marshal(chatCompletionRequest{
		Model:          c.model,
		Messages:       args.Messages,
		Temperature:    c.temperature,
		MaxTokens:      c.maxTokens,
		Seed:           c.seed,
		ResponseFormat: c.responseFormatFor(args),
	})
	if err != nil {
		return llmResult{}, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llmResult{}, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if c.organization != "" {
		req.Header.Set("OpenAI-Organization", c.organization)
	}
	if c.project != "" {
		req.Header.Set("OpenAI-Project", c.project)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return llmResult{}, true, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmResult{}, true, err
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(data, &completion); err != nil {
		return llmResult{}, false, fmt.Errorf("decode chat completion: %w", err)
	}
	if completion.Error != nil {
		return llmResult{}, retryStatus(resp.StatusCode), fmt.Errorf("chat completion: %s", completion.Error.Message)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return llmResult{}, retryStatus(resp.StatusCode), fmt.Errorf("chat completion status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(completion.Choices) == 0 {
		return llmResult{}, false, errors.New("chat completion returned no choices")
	}

	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return llmResult{}, false, errors.New("chat completion returned empty content")
	}
	return llmResult{Content: content}, false, nil
}

func (c *openAIChatClient) responseFormatFor(args llmArgs) *chatResponseFormat {
	if !c.responseFormat || !args.JSON {
		return nil
	}
	return &chatResponseFormat{Type: "json_object"}
}

func (c *openAIChatClient) backoff(attempt int) time.Duration {
	if c.backoffBase <= 0 {
		return 0
	}
	return c.backoffBase << attempt
}

func retryStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func optionalFloat(name string) (*float64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &parsed, nil
}

func optionalIntValue(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s: must be >= 0", name)
	}
	return parsed, nil
}

func optionalDurationSeconds(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("%s: must be > 0", name)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func optionalBoolValue(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}

func optionalInt(name string) (*int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &parsed, nil
}
