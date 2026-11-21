package acpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// EventKind classifies a streamed update from an external agent.
type EventKind int

const (
	// EventText is a chunk of the agent's reply text.
	EventText EventKind = iota
	// EventThought is a chunk of the agent's reasoning.
	EventThought
	// EventToolCall announces a new tool call (Title, ToolCallID).
	EventToolCall
	// EventToolUpdate reports a tool call status change (Status).
	EventToolUpdate
	// EventPlan carries the agent's plan, one entry per line in Text.
	EventPlan
	// EventNotice is a glaude-side note (session start, restart, warnings).
	EventNotice
)

// Event is one streamed update, delivered to the Prompt caller's callback.
type Event struct {
	Kind       EventKind
	Text       string
	ToolCallID string
	Title      string
	Status     string
}

// Result is the outcome of one prompt turn.
type Result struct {
	// Text is the agent's reply text for this turn.
	Text string
	// StopReason is the ACP stop reason (end_turn, cancelled, ...).
	StopReason string
	// NewSession reports that this turn started a fresh agent session (first
	// use, or a restart after the previous process died — context was lost).
	NewSession bool
	// Restarted reports that a previous session for this agent died and was
	// replaced, so the agent no longer remembers earlier turns.
	Restarted bool
}

// Options configures a Manager.
type Options struct {
	// Permission decides the agents' permission requests. nil rejects all.
	Permission PermissionFunc
	// Start launches an agent process. nil uses ExecStart.
	Start StartFunc
	// Logf receives diagnostics (never the terminal). nil discards.
	Logf func(format string, args ...any)
	// StartupTimeout bounds initialize + session/new. 0 means 2 minutes.
	StartupTimeout time.Duration
	// ClientVersion is reported to agents in initialize.
	ClientVersion string
}

// Manager owns the long-lived ACP sessions of external agents. One process
// and one session exist per (agent, working directory); prompts to the same
// agent are serialized and share the session's context for the lifetime of
// the Manager.
type Manager struct {
	specs map[string]AgentSpec
	opts  Options

	baseCtx context.Context
	cancel  context.CancelFunc

	mu     sync.Mutex
	slots  map[slotKey]*slot
	closed bool
}

type slotKey struct{ name, cwd string }

// slot serializes startup and prompts for one (agent, cwd).
type slot struct {
	mu      sync.Mutex
	sess    *session
	started bool // a session existed before (restart detection)
}

// NewManager creates a Manager for the given agent specs.
func NewManager(specs map[string]AgentSpec, opts Options) *Manager {
	if opts.Start == nil {
		opts.Start = ExecStart
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = 2 * time.Minute
	}
	if opts.ClientVersion == "" {
		opts.ClientVersion = "dev"
	}
	ctx, cancel := context.WithCancel(context.Background())
	cp := make(map[string]AgentSpec, len(specs))
	for k, v := range specs {
		if v.Name == "" {
			v.Name = k
		}
		cp[k] = v
	}
	return &Manager{specs: cp, opts: opts, baseCtx: ctx, cancel: cancel, slots: make(map[slotKey]*slot)}
}

// Has reports whether name is a configured external agent.
func (m *Manager) Has(name string) bool {
	if m == nil {
		return false
	}
	_, ok := m.specs[name]
	return ok
}

// Spec returns the spec of a configured agent.
func (m *Manager) Spec(name string) (AgentSpec, bool) {
	if m == nil {
		return AgentSpec{}, false
	}
	s, ok := m.specs[name]
	return s, ok
}

// Names returns the configured agent names in lexical order.
func (m *Manager) Names() []string {
	if m == nil {
		return nil
	}
	return SortedNames(m.specs)
}

// ErrClosed is returned by Prompt after Close.
var ErrClosed = errors.New("acpclient: manager closed")

// Prompt sends text to the named agent's session for cwd, starting the agent
// on first use (or after its process died), and streams updates to onEvent.
// Prompts to the same session are serialized.
func (m *Manager) Prompt(ctx context.Context, name, cwd, text string, onEvent func(Event)) (Result, error) {
	spec, ok := m.Spec(name)
	if !ok {
		return Result{}, fmt.Errorf("acpclient: unknown agent %q", name)
	}
	if onEvent == nil {
		onEvent = func(Event) {}
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Result{}, ErrClosed
	}
	key := slotKey{name, cwd}
	sl := m.slots[key]
	if sl == nil {
		sl = &slot{}
		m.slots[key] = sl
	}
	m.mu.Unlock()

	sl.mu.Lock()
	defer sl.mu.Unlock()

	var res Result
	if sl.sess == nil || !sl.sess.alive() {
		if sl.sess != nil {
			sl.sess.close()
		}
		res.NewSession = true
		res.Restarted = sl.started
		if res.Restarted {
			onEvent(Event{Kind: EventNotice, Text: name + " process exited; starting a new session (previous context lost)"})
		} else {
			onEvent(Event{Kind: EventNotice, Text: "Starting " + name})
		}
		s, err := m.startSession(ctx, spec, cwd, onEvent)
		if err != nil {
			return res, err
		}
		m.mu.Lock()
		closed := m.closed
		m.mu.Unlock()
		if closed {
			s.close()
			return res, ErrClosed
		}
		sl.sess, sl.started = s, true
	}

	text, stop, err := sl.sess.prompt(ctx, text, onEvent)
	res.Text, res.StopReason = text, stop
	if err != nil && !sl.sess.alive() {
		err = fmt.Errorf("%w%s", err, sl.sess.stderrHint())
	}
	return res, err
}

// Close terminates every agent process. It is safe to call more than once.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	slots := make([]*slot, 0, len(m.slots))
	for _, s := range m.slots {
		slots = append(slots, s)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, sl := range slots {
		wg.Add(1)
		go func(sl *slot) {
			defer wg.Done()
			// A running prompt holds sl.mu; cancelling the base context kills
			// the process, which ends the prompt and releases the lock.
			sl.mu.Lock()
			defer sl.mu.Unlock()
			if sl.sess != nil {
				sl.sess.close()
			}
		}(sl)
	}
	go func() {
		// Processes still busy after a grace period are killed.
		time.Sleep(closeGrace + time.Second)
		m.cancel()
	}()
	wg.Wait()
	m.cancel()
	return nil
}

func (m *Manager) startSession(ctx context.Context, spec AgentSpec, cwd string, onEvent func(Event)) (*session, error) {
	proc, err := m.opts.Start(m.baseCtx, spec, cwd)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.Name, err)
	}
	s := &session{spec: spec, cwd: cwd, proc: proc, perm: m.opts.Permission, logf: m.opts.Logf}
	s.conn = acp.NewClientSideConnection(&client{s: s}, proc.Stdin, proc.Stdout)
	s.conn.SetLogger(slog.New(logfHandler{logf: m.opts.Logf, prefix: "acp " + spec.Name + ": "}))

	sctx, cancel := context.WithTimeout(ctx, m.opts.StartupTimeout)
	defer cancel()
	fail := func(step string, err error) (*session, error) {
		hint := s.stderrHint()
		s.close()
		return nil, fmt.Errorf("%s %s: %w%s", spec.Name, step, err, hint)
	}

	initResp, err := s.conn.Initialize(sctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo:      &acp.Implementation{Name: "glaude", Version: m.opts.ClientVersion},
	})
	if err != nil {
		return fail("initialize", err)
	}
	if initResp.ProtocolVersion != acp.ProtocolVersionNumber {
		return fail("initialize", fmt.Errorf("unsupported protocol version %d", initResp.ProtocolVersion))
	}

	req := acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}}
	if len(spec.SessionMeta) > 0 {
		meta, merr := acp.NewMeta(spec.SessionMeta)
		if merr != nil {
			return fail("session/new", merr)
		}
		req.Meta = meta
	}
	newResp, err := s.conn.NewSession(sctx, req)
	if err != nil {
		if isAuthRequired(err) {
			return fail("session/new", fmt.Errorf("%w (sign in to %s first, outside glaude)", err, spec.Command))
		}
		return fail("session/new", err)
	}
	s.id = newResp.SessionId

	if spec.Model != "" {
		if merr := s.setModel(sctx, spec.Model); merr != nil {
			onEvent(Event{Kind: EventNotice, Text: fmt.Sprintf("could not set %s model %q: %v (using the agent's default)", spec.Name, spec.Model, merr)})
		}
	}
	m.opts.Logf("acp %s: session %s started in %s", spec.Name, s.id, cwd)
	return s, nil
}

// modelRetryDelays spaces the model switch attempts: right after session/new
// an agent may still be loading its model catalog (grok answers "unknown
// model id" on a cold start), so a rejected switch is retried.
var modelRetryDelays = []time.Duration{0, time.Second, 2 * time.Second}

func (s *session) setModel(ctx context.Context, model string) error {
	var err error
	for _, d := range modelRetryDelays {
		if d > 0 {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return errors.Join(err, ctx.Err())
			}
		}
		_, err = s.conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
			ValueId: &acp.SetSessionConfigOptionValueId{
				ConfigId:  "model",
				SessionId: s.id,
				Value:     acp.SessionConfigValueId(model),
			},
		})
		if err == nil {
			return nil
		}
		var re *acp.RequestError
		if !errors.As(err, &re) {
			return err // transport failure: retrying will not help
		}
	}
	return err
}

func isAuthRequired(err error) bool {
	var re *acp.RequestError
	return errors.As(err, &re) && re.Code == -32000
}

// session is one live agent process with one ACP session.
type session struct {
	spec AgentSpec
	cwd  string
	proc *Proc
	conn *acp.ClientSideConnection
	id   acp.SessionId
	perm PermissionFunc
	logf func(string, ...any)

	mu        sync.Mutex
	sink      func(Event)
	promptCtx context.Context
	reply     strings.Builder
	titles    map[acp.ToolCallId]string
	closeOnce sync.Once
}

func (s *session) alive() bool {
	select {
	case <-s.conn.Done():
		return false
	case <-s.proc.Done():
		return false
	default:
		return true
	}
}

func (s *session) prompt(ctx context.Context, text string, onEvent func(Event)) (string, string, error) {
	s.mu.Lock()
	s.sink, s.promptCtx = onEvent, ctx
	s.reply.Reset()
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.sink, s.promptCtx = nil, nil
		s.mu.Unlock()
	}()

	resp, err := s.conn.Prompt(ctx, acp.PromptRequest{
		SessionId: s.id,
		Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
	})
	s.mu.Lock()
	reply := s.reply.String()
	s.mu.Unlock()
	if err != nil {
		return reply, "", err
	}
	return reply, string(resp.StopReason), nil
}

func (s *session) emit(e Event) {
	s.mu.Lock()
	sink := s.sink
	s.mu.Unlock()
	if sink != nil {
		sink(e)
	}
}

func (s *session) currentCtx(fallback context.Context) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.promptCtx != nil {
		return s.promptCtx
	}
	return fallback
}

func (s *session) stderrHint() string {
	tail := strings.TrimSpace(s.proc.StderrTail())
	if tail == "" {
		return ""
	}
	return "\nagent stderr (tail):\n" + tail
}

func (s *session) close() {
	s.closeOnce.Do(func() { s.proc.Close() })
}

// logfHandler adapts the SDK's slog output to Options.Logf so nothing is
// written to the terminal (stderr would corrupt the TUI).
type logfHandler struct {
	logf   func(string, ...any)
	prefix string
	attrs  []slog.Attr
}

func (h logfHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h logfHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	for _, a := range h.attrs {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})
	h.logf("%s%s", h.prefix, b.String())
	return nil
}

func (h logfHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return h
}

func (h logfHandler) WithGroup(string) slog.Handler { return h }

var _ io.Closer = (*Proc)(nil)
