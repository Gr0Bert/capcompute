package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultAgentRunID    = "agent-1"
	defaultAgentQuestion = "What does capcompute make possible?"
	defaultAgentMaxSteps = 6
	defaultAgentMaxTicks = 12
	defaultAgentTrace    = "text"
)

type agentConfig struct {
	RunID    string
	Question string
	MaxSteps int
	MaxTicks int
	WasmPath string
	Trace    string
}

type traceWriter struct {
	out  io.Writer
	json bool
}

func agentConfigFromEnv() (agentConfig, error) {
	maxSteps, err := envInt("AGENT_MAX_STEPS", defaultAgentMaxSteps)
	if err != nil {
		return agentConfig{}, err
	}
	maxTicks, err := envInt("AGENT_MAX_TICKS", defaultAgentMaxTicks)
	if err != nil {
		return agentConfig{}, err
	}

	trace := envString("AGENT_TRACE", defaultAgentTrace)
	if trace != "text" && trace != "json" {
		return agentConfig{}, fmt.Errorf("AGENT_TRACE must be text or json")
	}

	return agentConfig{
		RunID:    envString("AGENT_RUN_ID", defaultAgentRunID),
		Question: envString("AGENT_QUESTION", defaultAgentQuestion),
		MaxSteps: maxSteps,
		MaxTicks: maxTicks,
		WasmPath: envString("AGENT_WASM_PATH", filepath.Join("examples", "agent", "agent.wasm")),
		Trace:    trace,
	}, nil
}

func llmFromEnv() (llmCompleter, error) {
	mode := envString("AGENT_LLM", "openai")
	switch mode {
	case "fake":
		return fakeLLM{}, nil
	case "openai":
		return openAIClientFromEnv()
	default:
		return nil, fmt.Errorf("AGENT_LLM must be fake or openai")
	}
}

func newTraceWriter(out io.Writer, format string) traceWriter {
	return traceWriter{out: out, json: format == "json"}
}

func (w traceWriter) Event(name string, fields ...any) {
	if w.json {
		w.JSONEvent(name, fields...)
		return
	}
	w.TextEvent(name, fields...)
}

func (w traceWriter) TextEvent(name string, fields ...any) {
	values := eventMap(fields...)
	switch name {
	case "agent.start":
		fmt.Fprintf(w.out, "agent: run=%s\n", values["run"])
	case "agent.question":
		fmt.Fprintf(w.out, "question: %s\n", values["question"])
	case "agent.yielded":
		fmt.Fprintf(w.out, "tick %v: yielded on %s\n", values["tick"], values["call"])
	case "async.completed":
		fmt.Fprintf(w.out, "async %v: stored %s result\n", values["tick"], values["call"])
	case "model.output":
		fmt.Fprintf(w.out, "model %v: %s\n", values["tick"], values["content"])
	case "agent.completed":
		fmt.Fprintln(w.out, "agent: completed")
	case "agent.output":
		fmt.Fprintf(w.out, "output: %s\n", values["output"])
	default:
		fmt.Fprintf(w.out, "%s: %v\n", name, values)
	}
}

func (w traceWriter) JSONEvent(name string, fields ...any) {
	values := eventMap(fields...)
	values["event"] = name
	data, err := json.Marshal(values)
	if err != nil {
		fmt.Fprintf(w.out, `{"event":"trace.error","message":%q}`+"\n", err.Error())
		return
	}
	fmt.Fprintln(w.out, string(data))
}

func eventMap(fields ...any) map[string]any {
	values := make(map[string]any)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		values[key] = fields[i+1]
	}
	return values
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be > 0", name)
	}
	return parsed, nil
}
