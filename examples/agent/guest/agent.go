//go:build tinygo

package main

import (
	"encoding/json"

	"github.com/extism/go-pdk"
)

//go:wasmimport extism:host/compute play
func hostPlay(uint64) uint64

type input struct {
	Message  string `json:"message"`
	MaxSteps int    `json:"max_steps,omitempty"`
}

type call struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type hostResponse struct {
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Message string          `json:"message,omitempty"`
}

type llmArgs struct {
	Messages []message `json:"messages"`
	JSON     bool      `json:"json,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmResult struct {
	Content string `json:"content"`
}

type action struct {
	Action string `json:"action"`
	Tool   string `json:"tool,omitempty"`
	Reason string `json:"reason,omitempty"`
	Query  string `json:"query,omitempty"`
	URL    string `json:"url,omitempty"`
	Note   string `json:"note,omitempty"`
	Answer string `json:"answer,omitempty"`
}

type searchArgs struct {
	Query string `json:"query"`
}

type readArgs struct {
	URL string `json:"url"`
}

type rememberArgs struct {
	Note string `json:"note"`
}

type toolObservation struct {
	Tool    string `json:"tool"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
	Results []hit  `json:"results,omitempty"`
	Content string `json:"content"`
	Note    string `json:"note"`
	Status  string `json:"status"`
	Score   int    `json:"score"`
}

type hit struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
	Score   int    `json:"score"`
}

type output struct {
	Answer  string         `json:"answer"`
	Steps   []outputStep   `json:"steps"`
	Sources []outputSource `json:"sources"`
}

type outputStep struct {
	Action string `json:"action"`
	Tool   string `json:"tool,omitempty"`
	Reason string `json:"reason,omitempty"`
	Query  string `json:"query,omitempty"`
	URL    string `json:"url,omitempty"`
	Note   string `json:"note,omitempty"`
}

type outputSource struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

//go:wasmexport run
func run() int32 {
	var in input
	if err := pdk.InputJSON(&in); err != nil {
		pdk.SetError(err)
		return 1
	}
	if in.MaxSteps <= 0 {
		in.MaxSteps = 4
	}

	transcript := []message{
		{Role: "system", Content: `You are an autonomous capcompute agent.
Available tools:
- tool.search: search the local capcompute knowledge base. Args: {"query":"short search query"}.
- tool.read: read a source returned by search. Args: {"url":"source url"}.
- tool.remember: store a short note for later reasoning. Args: {"note":"short note"}.
On each turn choose exactly one action.
Return only compact JSON.
For search: {"action":"tool","tool":"tool.search","reason":"why","query":"search query"}.
For reading: {"action":"tool","tool":"tool.read","reason":"why","url":"source url"}.
For notes: {"action":"tool","tool":"tool.remember","reason":"why","note":"short note"}.
For final answer: {"action":"final","reason":"why done","answer":"answer grounded in tool observations"}.
Use at most the available tool observations. Do not invent sources.`},
		{Role: "user", Content: in.Message},
	}
	var steps []outputStep
	var sources []outputSource

	for i := 0; i < in.MaxSteps; i++ {
		next, completion, ok := decide(transcript)
		if !ok {
			return 0
		}
		transcript = append(transcript, message{Role: "assistant", Content: completion.Content})

		switch next.Action {
		case "final":
			if next.Answer == "" {
				pdk.SetErrorString("model returned empty final answer")
				return 1
			}
			steps = append(steps, outputStep{Action: "final", Reason: next.Reason})
			if err := pdk.OutputJSON(output{
				Answer:  next.Answer,
				Steps:   steps,
				Sources: sources,
			}); err != nil {
				pdk.SetError(err)
				return 1
			}
			return 0
		case "tool":
			toolCall, err := buildToolCall(next)
			if err != "" {
				pdk.SetErrorString(err)
				return 1
			}

			result, ok := dispatchJSON(next.Tool, toolCall)
			if !ok {
				return 0
			}

			var observation toolObservation
			if err := json.Unmarshal(result.Result, &observation); err != nil {
				pdk.SetError(err)
				return 1
			}
			steps = append(steps, outputStep{
				Action: "tool",
				Tool:   next.Tool,
				Reason: next.Reason,
				Query:  next.Query,
				URL:    next.URL,
				Note:   next.Note,
			})
			sources = appendObservationSource(sources, observation)
			transcript = append(transcript, message{Role: "tool", Content: mustJSON(observation)})
		default:
			pdk.SetErrorString("model returned unsupported action: " + next.Action)
			return 1
		}
	}

	pdk.SetErrorString("agent reached max steps without final answer")
	return 1
}

func buildToolCall(next action) (any, string) {
	switch next.Tool {
	case "tool.search":
		if next.Query == "" {
			return nil, "model returned empty search query"
		}
		return searchArgs{Query: next.Query}, ""
	case "tool.read":
		if next.URL == "" {
			return nil, "model returned empty read url"
		}
		return readArgs{URL: next.URL}, ""
	case "tool.remember":
		if next.Note == "" {
			return nil, "model returned empty note"
		}
		return rememberArgs{Note: next.Note}, ""
	default:
		return nil, "model selected unsupported tool: " + next.Tool
	}
}

func decide(transcript []message) (action, llmResult, bool) {
	response, ok := dispatchJSON("llm.complete", llmArgs{JSON: true, Messages: transcript})
	if !ok {
		return action{}, llmResult{}, false
	}

	var completion llmResult
	if err := json.Unmarshal(response.Result, &completion); err != nil {
		pdk.SetError(err)
		return action{}, llmResult{}, false
	}

	var next action
	if err := json.Unmarshal(extractJSONObject(completion.Content), &next); err != nil {
		pdk.SetError(err)
		return action{}, llmResult{}, false
	}
	return next, completion, true
}

func appendSource(sources []outputSource, source outputSource) []outputSource {
	for _, existing := range sources {
		if cleanURL(existing.URL) == cleanURL(source.URL) {
			return sources
		}
	}
	return append(sources, source)
}

func appendObservationSource(sources []outputSource, observation toolObservation) []outputSource {
	if observation.URL == "" || observation.Title == "" {
		return sources
	}
	snippet := observation.Snippet
	if snippet == "" {
		snippet = observation.Content
	}
	return appendSource(sources, outputSource{
		Title:   observation.Title,
		URL:     cleanURL(observation.URL),
		Snippet: snippet,
	})
}

func cleanURL(raw string) string {
	for i := 0; i < len(raw); i++ {
		if raw[i] == '?' {
			return raw[:i]
		}
	}
	return raw
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		pdk.SetError(err)
		return ""
	}
	return string(data)
}

func extractJSONObject(content string) []byte {
	start := -1
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(content); i++ {
		c := content[i]
		if start == -1 {
			if c == '{' {
				start = i
				depth = 1
			}
			continue
		}

		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return []byte(content[start : i+1])
			}
		}
	}
	return []byte(content)
}

func dispatchJSON(name string, args any) (hostResponse, bool) {
	data, err := json.Marshal(args)
	if err != nil {
		pdk.SetError(err)
		return hostResponse{}, false
	}

	req, err := json.Marshal(call{Name: name, Args: data})
	if err != nil {
		pdk.SetError(err)
		return hostResponse{}, false
	}

	mem := pdk.AllocateBytes(req)
	defer mem.Free()

	result := pdk.FindMemory(hostPlay(mem.Offset()))
	var response hostResponse
	if err := json.Unmarshal(result.ReadBytes(), &response); err != nil {
		pdk.SetError(err)
		return hostResponse{}, false
	}

	switch response.Status {
	case "result":
		return response, true
	case "yield":
		return response, false
	default:
		if response.Message == "" {
			response.Message = "host call failed"
		}
		pdk.SetErrorString(response.Message)
		return hostResponse{}, false
	}
}

func main() {}
