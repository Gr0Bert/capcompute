package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentDemoYieldsAndReplays(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo is required to build the guest wasm")
	}

	wasmPath := filepath.Join(t.TempDir(), "agent.wasm")
	cmd := exec.Command(
		"tinygo",
		"build",
		"-target", "wasip1",
		"-buildmode", "c-shared",
		"-tags", "tinygo",
		"-o", wasmPath,
		"agent.go",
	)
	cmd.Dir = filepath.Join("guest")
	cmd.Env = append(cmd.Environ(), "XDG_CACHE_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build guest wasm: %v\n%s", err, output)
	}

	var out bytes.Buffer
	config := agentConfig{
		RunID:    defaultAgentRunID,
		Question: defaultAgentQuestion,
		MaxSteps: defaultAgentMaxSteps,
		MaxTicks: defaultAgentMaxTicks,
		WasmPath: wasmPath,
		Trace:    defaultAgentTrace,
	}
	if err := runDemo(context.Background(), &out, config, fakeLLM{}); err != nil {
		t.Fatalf("run demo: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"agent: run=agent-1",
		"question: What does capcompute make possible?",
		"tick 1: yielded on llm.complete",
		"async 1: stored llm.complete result",
		"model 1:",
		"tick 2: yielded on llm.complete",
		"async 2: stored llm.complete result",
		"model 2:",
		"tick 3: yielded on llm.complete",
		"async 3: stored llm.complete result",
		"model 3:",
		"tick 4: yielded on llm.complete",
		"async 4: stored llm.complete result",
		"model 4:",
		"agent: completed",
		`"steps":[{"action":"tool","tool":"tool.search"`,
		`{"action":"tool","tool":"tool.read"`,
		`{"action":"tool","tool":"tool.remember"`,
		`"sources":[{"title":"capcompute runtime notes"`,
		`"url":"memory://capcompute/runtime-notes`,
		"autonomous, resumable Wasm agents",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}
