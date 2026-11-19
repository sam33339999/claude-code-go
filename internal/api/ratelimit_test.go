package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// rateLimiter
// ─────────────────────────────────────────────────────────────────────────────

func TestRateLimiter_Spacing(t *testing.T) {
	lim := &rateLimiter{interval: 80 * time.Millisecond}
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := lim.acquire(ctx); err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// First slot immediate, then two intervals.
	if elapsed < 150*time.Millisecond {
		t.Errorf("3 acquires took %v, want >= ~160ms (strict spacing)", elapsed)
	}
}

func TestRateLimiter_FirstImmediate(t *testing.T) {
	lim := &rateLimiter{interval: time.Hour}
	start := time.Now()
	if err := lim.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("first acquire must be immediate")
	}
}

func TestRateLimiter_CtxCancelWhileWaiting(t *testing.T) {
	lim := &rateLimiter{interval: time.Hour}
	ctx := context.Background()
	if err := lim.acquire(ctx); err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := lim.acquire(cancelCtx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Error("cancellation must abort the wait promptly")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// rateLimitedClient
// ─────────────────────────────────────────────────────────────────────────────

// trackingClient records in-flight concurrency; streams hold their slot until
// the returned reader is closed.
type trackingClient struct {
	mu       sync.Mutex
	inflight int
	peak     int
	failNext bool
}

func (c *trackingClient) enter() {
	c.mu.Lock()
	c.inflight++
	if c.inflight > c.peak {
		c.peak = c.inflight
	}
	c.mu.Unlock()
}

func (c *trackingClient) leave() {
	c.mu.Lock()
	c.inflight--
	c.mu.Unlock()
}

type trackingReader struct{ leave func() }

func (r *trackingReader) Next() (*StreamEvent, error) { return nil, io.EOF }
func (r *trackingReader) Close() error                { r.leave(); return nil }

func (c *trackingClient) Stream(_ context.Context, _ *MessageRequest) (StreamReader, error) {
	c.mu.Lock()
	fail := c.failNext
	c.failNext = false
	c.mu.Unlock()
	if fail {
		return nil, &APIError{StatusCode: 500, Message: "boom"}
	}
	c.enter()
	return &trackingReader{leave: c.leave}, nil
}

func (c *trackingClient) Complete(_ context.Context, _ *MessageRequest) (*MessageResponse, error) {
	c.enter()
	defer c.leave()
	return &MessageResponse{ID: "ok"}, nil
}

func TestRateLimitedClient_ConcurrencyCap(t *testing.T) {
	inner := &trackingClient{}
	c := applyRatePolicy(inner, RatePolicy{MaxConcurrent: 2}).(*rateLimitedClient)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader, err := c.Stream(context.Background(), &MessageRequest{})
			if err != nil {
				t.Errorf("Stream: %v", err)
				return
			}
			time.Sleep(20 * time.Millisecond) // hold the slot
			_ = reader.Close()
		}()
	}
	wg.Wait()

	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.peak > 2 {
		t.Errorf("peak concurrency = %d, want <= 2", inner.peak)
	}
	if inner.inflight != 0 {
		t.Errorf("inflight after all closes = %d, want 0 (slots must be released)", inner.inflight)
	}
}

func TestRateLimitedClient_ReleaseOnError(t *testing.T) {
	inner := &trackingClient{failNext: true}
	c := applyRatePolicy(inner, RatePolicy{MaxConcurrent: 1}).(*rateLimitedClient)

	if _, err := c.Stream(context.Background(), &MessageRequest{}); err == nil {
		t.Fatal("expected error from inner client")
	}
	// Slot must have been released: the next call must not deadlock.
	done := make(chan struct{})
	go func() {
		reader, err := c.Stream(context.Background(), &MessageRequest{})
		if err == nil {
			_ = reader.Close()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrency slot leaked after inner error")
	}
}

func TestRateLimitedClient_CompleteReleases(t *testing.T) {
	inner := &trackingClient{}
	c := applyRatePolicy(inner, RatePolicy{MaxConcurrent: 1})
	for i := 0; i < 3; i++ {
		if _, err := c.Complete(context.Background(), &MessageRequest{}); err != nil {
			t.Fatalf("Complete %d: %v", i, err)
		}
	}
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.inflight != 0 {
		t.Errorf("inflight = %d, want 0", inner.inflight)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Policy config
// ─────────────────────────────────────────────────────────────────────────────

func TestRatePolicyFromEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_API_RPM", "60")
	t.Setenv("CLAUDE_CODE_API_MAX_CONCURRENT", "3")
	p := ratePolicyFromEnv()
	if p.RPM != 60 || p.MaxConcurrent != 3 {
		t.Errorf("policy = %+v, want RPM 60 / MaxConcurrent 3", p)
	}
}

func TestRatePolicyFromEnv_InvalidIgnored(t *testing.T) {
	t.Setenv("CLAUDE_CODE_API_RPM", "not-a-number")
	t.Setenv("CLAUDE_CODE_API_MAX_CONCURRENT", "-5")
	p := ratePolicyFromEnv()
	if p.RPM != 0 || p.MaxConcurrent != 0 {
		t.Errorf("policy = %+v, want zero (unlimited)", p)
	}
}

func TestNewClient_AppliesRatePolicy(t *testing.T) {
	t.Setenv("CLAUDE_CODE_API_RPM", "")
	t.Setenv("CLAUDE_CODE_API_MAX_CONCURRENT", "")

	// Explicit policy wraps.
	c, err := NewClient(ClientConfig{Provider: ProviderOpenAI, APIKey: "k", RatePolicy: RatePolicy{RPM: 60}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*rateLimitedClient); !ok {
		t.Errorf("client type %T, want *rateLimitedClient when policy set", c)
	}

	// No policy, no env → unwrapped.
	c2, err := NewClient(ClientConfig{Provider: ProviderOpenAI, APIKey: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c2.(*rateLimitedClient); ok {
		t.Error("client must stay unwrapped with no policy")
	}

	// Env fallback wraps.
	t.Setenv("CLAUDE_CODE_API_MAX_CONCURRENT", "2")
	c3, err := NewClient(ClientConfig{Provider: ProviderOpenAI, APIKey: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c3.(*rateLimitedClient); !ok {
		t.Errorf("client type %T, want wrapped via env policy", c3)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// WithRetry auth semantics
// ─────────────────────────────────────────────────────────────────────────────

func TestWithRetry_401FailsFastWithoutRefreshHook(t *testing.T) {
	attempts := 0
	_, err := WithRetry(context.Background(), func(_ context.Context, _ int) (string, error) {
		attempts++
		return "", &APIError{StatusCode: 401, Message: "unauthorized", Headers: http.Header{}}
	}, RetryOptions{QuerySource: "foreground"})
	var cannot *CannotRetryError
	if !errors.As(err, &cannot) {
		t.Fatalf("err = %v, want CannotRetryError", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no backoff ladder on auth failure)", attempts)
	}
}

func TestWithRetry_429RetriesThenSucceeds(t *testing.T) {
	attempts := 0
	got, err := WithRetry(context.Background(), func(_ context.Context, _ int) (string, error) {
		attempts++
		if attempts == 1 {
			return "", &APIError{StatusCode: 429, Message: "slow down", Headers: http.Header{"Retry-After": {"0"}}}
		}
		return "ok", nil
	}, RetryOptions{QuerySource: "foreground"})
	if err != nil {
		t.Fatalf("WithRetry: %v", err)
	}
	if got != "ok" || attempts != 2 {
		t.Errorf("got %q after %d attempts, want ok after 2", got, attempts)
	}
}
