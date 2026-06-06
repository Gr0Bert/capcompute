package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOpenAIChatClientCompletes(t *testing.T) {
	var request chatCompletionRequest
	client := &openAIChatClient{
		baseURL:        "https://openai-compatible.test/v1",
		apiKey:         "test-key",
		organization:   "org-1",
		project:        "proj-1",
		model:          "test-model",
		seed:           intPtr(42),
		maxRetries:     0,
		responseFormat: true,
		http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", req.Method)
			}
			if req.URL.String() != "https://openai-compatible.test/v1/chat/completions" {
				t.Fatalf("url = %s", req.URL.String())
			}
			if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("authorization = %q", got)
			}
			if got := req.Header.Get("OpenAI-Organization"); got != "org-1" {
				t.Fatalf("organization = %q", got)
			}
			if got := req.Header.Get("OpenAI-Project"); got != "proj-1" {
				t.Fatalf("project = %q", got)
			}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			return jsonResponse(http.StatusOK, `{
				"choices": [
					{"message": {"role": "assistant", "content": "model answer"}}
				]
			}`), nil
		})},
	}

	result, err := client.Complete(context.Background(), llmArgs{
		Messages: []chatMessage{{Role: "user", Content: "hello"}},
		JSON:     true,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Content != "model answer" {
		t.Fatalf("content = %q", result.Content)
	}
	if request.Model != "test-model" {
		t.Fatalf("model = %q", request.Model)
	}
	if len(request.Messages) != 1 || request.Messages[0].Content != "hello" {
		t.Fatalf("messages = %#v", request.Messages)
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v", request.ResponseFormat)
	}
	if request.Seed == nil || *request.Seed != 42 {
		t.Fatalf("seed = %#v", request.Seed)
	}
}

func TestOpenAIChatClientReportsAPIError(t *testing.T) {
	client := &openAIChatClient{
		baseURL:    "https://openai-compatible.test/v1",
		apiKey:     "test-key",
		model:      "test-model",
		maxRetries: 0,
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadRequest, `{"error":{"message":"bad request"}}`), nil
		})},
	}

	_, err := client.Complete(context.Background(), llmArgs{
		Messages: []chatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenAIChatClientRetriesTransientErrors(t *testing.T) {
	calls := 0
	client := &openAIChatClient{
		baseURL:     "https://openai-compatible.test/v1",
		apiKey:      "test-key",
		model:       "test-model",
		maxRetries:  1,
		backoffBase: 0,
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return jsonResponse(http.StatusTooManyRequests, `{"error":{"message":"try again"}}`), nil
			}
			return jsonResponse(http.StatusOK, `{
				"choices": [
					{"message": {"role": "assistant", "content": "retried answer"}}
				]
			}`), nil
		})},
	}

	result, err := client.Complete(context.Background(), llmArgs{
		Messages: []chatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Content != "retried answer" {
		t.Fatalf("content = %q", result.Content)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func intPtr(v int) *int {
	return &v
}
