package main

import (
	"capcompute"
	"capcompute/dispatcher"
	hostdispatcher "capcompute/dispatcher/host"
	"capcompute/dispatcher/replay"
	"capcompute/dispatcher/replay/tape/journaled"
	"capcompute/dispatcher/replay/tape/journaled/journal/memory"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	extism "github.com/extism/go-sdk"
)

type agentRun struct {
	ID string
}

func (r agentRun) SessionKey() string {
	return r.ID
}

type input struct {
	Message  string `json:"message"`
	MaxSteps int    `json:"max_steps,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmArgs struct {
	Messages []chatMessage `json:"messages"`
	JSON     bool          `json:"json,omitempty"`
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

type runNotes struct {
	notes map[string][]string
}

type agentHandlers struct {
	notes *runNotes
}

func (h agentHandlers) Execute(_ context.Context, run agentRun, call dispatcher.Call) (dispatcher.Outcome, error) {
	switch call.Name {
	case "llm.complete":
		return dispatcher.Yield("waiting for fake llm"), nil
	case "tool.search":
		var args searchArgs
		if err := json.Unmarshal(call.Args, &args); err != nil {
			return dispatcher.Outcome{}, err
		}
		return jsonResult(fakeSearch(args.Query))
	case "tool.read":
		var args readArgs
		if err := json.Unmarshal(call.Args, &args); err != nil {
			return dispatcher.Outcome{}, err
		}
		return jsonResult(readDocument(args.URL))
	case "tool.remember":
		var args rememberArgs
		if err := json.Unmarshal(call.Args, &args); err != nil {
			return dispatcher.Outcome{}, err
		}
		h.notes.remember(run.SessionKey(), args.Note)
		return jsonResult(toolObservation{
			Tool:   "tool.remember",
			Note:   args.Note,
			Status: "stored",
		})
	default:
		return dispatcher.Failed("unknown call: " + call.Name), nil
	}
}

type dispatcherFactory struct {
	journal *memory.Journal
	notes   *runNotes
}

func (f dispatcherFactory) NewDispatcher(context.Context, agentRun) (dispatcher.Dispatcher[agentRun], error) {
	tape := journaled.NewTape(f.journal)
	next := &hostdispatcher.Dispatcher[agentRun]{Handlers: agentHandlers{notes: f.notes}}
	return replay.NewDispatcher[agentRun](tape, next), nil
}

func main() {
	config, err := agentConfigFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	llm, err := llmFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := runDemo(context.Background(), os.Stdout, config, llm); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDemo(ctx context.Context, out io.Writer, config agentConfig, llm llmCompleter) error {
	trace := newTraceWriter(out, config.Trace)
	run := agentRun{ID: config.RunID}
	journal := memory.NewJournal()
	notes := newRunNotes()
	compute, err := capcompute.NewComputeCompiledPlugin[string, agentRun](ctx, capcompute.Config[string, agentRun]{
		Manifest: extism.Manifest{Wasm: []extism.Wasm{extism.WasmFile{Path: config.WasmPath}}},
		PluginConfig: extism.PluginConfig{
			EnableWasi: true,
		},
		Dispatchers: dispatcherFactory{journal: journal, notes: notes},
	})
	if err != nil {
		return err
	}
	defer compute.Close(ctx)

	trace.Event("agent.start", "run", run.ID)
	trace.Event("agent.question", "question", config.Question)

	results, err := compute.Play(ctx, run, playRequest(config.Question, config.MaxSteps))
	if err != nil {
		return err
	}

	for tick := 1; tick <= config.MaxTicks; tick++ {
		result := <-results
		if result.Err != nil {
			return result.Err
		}

		switch result.Status {
		case capcompute.PlayCompleted:
			trace.Event("agent.completed")
			trace.Event("agent.output", "output", json.RawMessage(result.Output))
			return nil
		case capcompute.PlayYielded:
			if result.Yielded == nil {
				return fmt.Errorf("tick %d yielded without a call", tick)
			}
			trace.Event("agent.yielded", "tick", tick, "call", result.Yielded.Name)
			completion, err := completeLLM(ctx, journal, llm, *result.Yielded)
			if err != nil {
				return err
			}
			trace.Event("async.completed", "tick", tick, "call", result.Yielded.Name)
			trace.Event("model.output", "tick", tick, "content", summarize(completion.Content, 160))
		default:
			return fmt.Errorf("tick %d status = %s", tick, result.Status)
		}

		results, err = compute.Replay(ctx, run.SessionKey())
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("agent exceeded max ticks: %d", config.MaxTicks)
}

func playRequest(message string, maxSteps int) capcompute.PlayRequest {
	data, err := json.Marshal(input{Message: message, MaxSteps: maxSteps})
	if err != nil {
		panic(err)
	}
	return capcompute.PlayRequest{Input: data}
}

type llmCompleter interface {
	Complete(ctx context.Context, args llmArgs) (llmResult, error)
}

func completeLLM(ctx context.Context, journal *memory.Journal, llm llmCompleter, call dispatcher.Call) (llmResult, error) {
	if call.Name != "llm.complete" {
		return llmResult{}, fmt.Errorf("cannot complete async call %q", call.Name)
	}

	var args llmArgs
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return llmResult{}, err
	}

	result, err := llm.Complete(ctx, args)
	if err != nil {
		return llmResult{}, err
	}

	outcome, err := jsonResult(result)
	if err != nil {
		return llmResult{}, err
	}
	return result, journal.Store(journal.Length(), call, outcome)
}

type fakeLLM struct{}

func (fakeLLM) Complete(_ context.Context, args llmArgs) (llmResult, error) {
	if _, ok := lastToolObservation(args.Messages, "tool.remember"); ok {
		document, _ := lastToolObservation(args.Messages, "tool.read")
		action := action{
			Action: "final",
			Reason: "The source was read and the key conclusion was stored in working memory.",
			Answer: "Capcompute can run autonomous, resumable Wasm agents. The guest chooses host tools, yields while async model work is pending, and replays completed calls without repeating side effects. Source: " + document.Title + ".",
		}
		return llmResult{Content: mustJSON(action)}, nil
	}
	if document, ok := lastToolObservation(args.Messages, "tool.read"); ok {
		action := action{
			Action: "tool",
			Tool:   "tool.remember",
			Reason: "Store the central claim before final synthesis.",
			Note:   "Capcompute supports resumable Wasm agents through host dispatch, yield, and replay. Source: " + document.Title,
		}
		return llmResult{Content: mustJSON(action)}, nil
	}
	if search, ok := lastToolObservation(args.Messages, "tool.search"); ok {
		next := action{
			Action: "tool",
			Tool:   "tool.read",
			Reason: "Read the strongest candidate returned by search.",
			URL:    search.URL,
		}
		return llmResult{Content: mustJSON(next)}, nil
	}

	user := lastContent(args.Messages, "user")
	next := action{
		Action: "tool",
		Tool:   "tool.search",
		Reason: "Gather local evidence before answering.",
		Query:  "capcompute autonomous resumable wasm agents " + strings.TrimSpace(user),
	}
	return llmResult{Content: mustJSON(next)}, nil
}

func fakeSearch(query string) toolObservation {
	results := rankDocuments(query)
	if len(results) == 0 {
		return toolObservation{
			Tool:    "tool.search",
			Title:   "No local result",
			Snippet: "The local search corpus did not contain a matching document.",
			URL:     "memory://capcompute/search?q=" + urlQuery(query),
		}
	}
	top := results[0]
	top.Results = hitsFromResults(results)
	return top
}

func readDocument(rawURL string) toolObservation {
	doc, ok := findDocument(rawURL)
	if !ok {
		return toolObservation{
			Tool:    "tool.read",
			Title:   "Document not found",
			URL:     rawURL,
			Content: "No local document exists for this URL.",
			Status:  "missing",
		}
	}
	return toolObservation{
		Tool:    "tool.read",
		Title:   doc.Title,
		Snippet: doc.Snippet,
		URL:     doc.URL,
		Content: doc.Content,
		Status:  "read",
	}
}

func lastToolObservation(messages []chatMessage, tool string) (toolObservation, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "tool" {
			continue
		}
		var observation toolObservation
		if err := json.Unmarshal([]byte(messages[i].Content), &observation); err != nil {
			return toolObservation{}, false
		}
		if observation.Tool != tool {
			continue
		}
		return observation, true
	}
	return toolObservation{}, false
}

func lastContent(messages []chatMessage, role string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return messages[i].Content
		}
	}
	return ""
}

type searchDocument struct {
	Title    string
	Snippet  string
	URL      string
	Content  string
	Keywords []string
}

var searchCorpus = []searchDocument{
	{
		Title:    "capcompute runtime notes",
		Snippet:  "capcompute runs deterministic Wasm guests over host capabilities, with replayed command results and yield/resume support for async work.",
		URL:      "memory://capcompute/runtime-notes",
		Content:  "capcompute compiles a Wasm guest once, creates per-run plugin sessions, and routes guest capability calls through dispatchers. A replay tape records completed command outcomes. When async work yields, the guest can later re-enter from the top and receive replayed outcomes without repeating side effects.",
		Keywords: []string{"capcompute", "wasm", "guest", "capability", "yield", "replay", "agent"},
	},
	{
		Title:    "Replay tape design",
		Snippet:  "The replay tape serves recorded command outcomes before delegating new calls, preventing duplicate side effects after a guest re-enters from the top.",
		URL:      "memory://capcompute/replay-tape",
		Content:  "The replay tape owns cursor state over a journal of call/outcome records. During replay it matches the next guest call against recorded history and returns the prior outcome. When the guest reaches a new call, dispatch continues upstream and successful results are appended.",
		Keywords: []string{"tape", "journal", "replay", "side effects", "deterministic"},
	},
	{
		Title:    "Dispatcher boundary",
		Snippet:  "Dispatchers own host capability policy and route guest calls to handlers such as model calls, search, APIs, and storage.",
		URL:      "memory://capcompute/dispatcher-boundary",
		Content:  "Dispatchers are composable host-call boundaries. A replay dispatcher can wrap a handler dispatcher; other decorators can add audit, policy, metrics, or tracing. The guest only sees capability calls and outcomes.",
		Keywords: []string{"dispatcher", "handler", "policy", "tool", "host"},
	},
}

func rankDocuments(query string) []toolObservation {
	terms := queryTerms(query)
	results := make([]toolObservation, 0, len(searchCorpus))
	for _, doc := range searchCorpus {
		score := scoreDocument(doc, terms)
		if score == 0 {
			continue
		}
		results = append(results, toolObservation{
			Tool:    "tool.search",
			Title:   doc.Title,
			Snippet: doc.Snippet,
			URL:     doc.URL + "?q=" + urlQuery(query),
			Score:   score,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func hitsFromResults(results []toolObservation) []hit {
	hits := make([]hit, 0, len(results))
	for _, result := range results {
		hits = append(hits, hit{
			Title:   result.Title,
			Snippet: result.Snippet,
			URL:     result.URL,
			Score:   result.Score,
		})
	}
	return hits
}

func findDocument(rawURL string) (searchDocument, bool) {
	cleanURL := strings.Split(rawURL, "?")[0]
	for _, doc := range searchCorpus {
		if doc.URL == cleanURL {
			return doc, true
		}
	}
	return searchDocument{}, false
}

func newRunNotes() *runNotes {
	return &runNotes{notes: make(map[string][]string)}
}

func (n *runNotes) remember(runID string, note string) {
	if n == nil {
		return
	}
	n.notes[runID] = append(n.notes[runID], note)
}

func scoreDocument(doc searchDocument, terms []string) int {
	haystack := strings.ToLower(doc.Title + " " + doc.Snippet + " " + strings.Join(doc.Keywords, " "))
	score := 0
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			score++
		}
	}
	return score
}

func queryTerms(query string) []string {
	seen := make(map[string]struct{})
	var terms []string
	for _, raw := range strings.Fields(strings.ToLower(query)) {
		term := strings.Trim(raw, ".,:;!?()[]{}\"'")
		if len(term) < 3 {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}
	return terms
}

func urlQuery(query string) string {
	return strings.ReplaceAll(strings.TrimSpace(query), " ", "+")
}

func summarize(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func jsonResult(v any) (dispatcher.Outcome, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return dispatcher.Outcome{}, err
	}
	return dispatcher.Result(data), nil
}
