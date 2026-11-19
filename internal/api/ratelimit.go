package api

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"
)

// RatePolicy configures client-side request pacing, applied uniformly to all
// providers at the Client boundary. Zero values mean unlimited.
type RatePolicy struct {
	// RPM caps requests per minute (strict spacing, burst 1). Example: RPM=60
	// spaces requests at least 1s apart. 0 disables pacing.
	RPM int
	// MaxConcurrent caps in-flight requests (a streaming request holds its
	// slot until the reader is closed). 0 disables the cap.
	MaxConcurrent int
}

// ratePolicyFromEnv reads the rate policy from environment variables:
// CLAUDE_CODE_API_RPM and CLAUDE_CODE_API_MAX_CONCURRENT. Invalid or unset
// values mean unlimited (the reactive WithRetry backoff still applies).
func ratePolicyFromEnv() RatePolicy {
	var p RatePolicy
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_API_RPM")); err == nil && v > 0 {
		p.RPM = v
	}
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_API_MAX_CONCURRENT")); err == nil && v > 0 {
		p.MaxConcurrent = v
	}
	return p
}

// applyRatePolicy wraps c with pacing/concurrency control when the policy is
// non-empty; otherwise it returns c unchanged.
func applyRatePolicy(c Client, p RatePolicy) Client {
	if p.RPM <= 0 && p.MaxConcurrent <= 0 {
		return c
	}
	var lim *rateLimiter
	if p.RPM > 0 {
		lim = &rateLimiter{interval: time.Minute / time.Duration(p.RPM)}
	}
	var sem chan struct{}
	if p.MaxConcurrent > 0 {
		sem = make(chan struct{}, p.MaxConcurrent)
	}
	return &rateLimitedClient{inner: c, rate: lim, sem: sem}
}

// rateLimiter enforces strict minimum spacing between request starts.
// mu guards next (the earliest allowed start of the next request).
type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// acquire reserves the next request slot, waiting until it arrives.
// Context cancellation aborts the wait (the reserved slot is not refunded;
// spacing stays conservative).
func (l *rateLimiter) acquire(ctx context.Context) error {
	if l == nil || l.interval <= 0 {
		return nil
	}
	l.mu.Lock()
	now := time.Now()
	slot := l.next
	if slot.Before(now) {
		slot = now
	}
	l.next = slot.Add(l.interval)
	l.mu.Unlock()

	wait := time.Until(slot)
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// rateLimitedClient is a provider-agnostic Client wrapper: every provider
// (direct/openai/bedrock/vertex/foundry) gets the same pacing and concurrency
// control without provider-specific code.
type rateLimitedClient struct {
	inner Client
	rate  *rateLimiter  // nil = no pacing
	sem   chan struct{} // nil = unlimited concurrency
}

// Compile-time assertion: the wrapper satisfies Client.
var _ Client = (*rateLimitedClient)(nil)

// acquire takes a concurrency slot (if capped) and then a pacing slot.
func (c *rateLimitedClient) acquire(ctx context.Context) error {
	if c.sem != nil {
		select {
		case c.sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := c.rate.acquire(ctx); err != nil {
		c.release()
		return err
	}
	return nil
}

// release returns the concurrency slot.
func (c *rateLimitedClient) release() {
	if c.sem != nil {
		<-c.sem
	}
}

// Stream acquires a slot, calls the inner client, and holds the concurrency
// slot until the returned reader is closed.
func (c *rateLimitedClient) Stream(ctx context.Context, req *MessageRequest) (StreamReader, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	reader, err := c.inner.Stream(ctx, req)
	if err != nil {
		c.release()
		return nil, err
	}
	return &releasedReader{StreamReader: reader, release: c.release}, nil
}

// Complete acquires a slot for the duration of the call.
func (c *rateLimitedClient) Complete(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()
	return c.inner.Complete(ctx, req)
}

// releasedReader releases the concurrency slot when the stream is closed
// (exactly once, even if Close is called repeatedly).
type releasedReader struct {
	StreamReader
	release func()
	once    sync.Once
}

// Close closes the underlying reader and releases the concurrency slot.
func (r *releasedReader) Close() error {
	err := r.StreamReader.Close()
	r.once.Do(r.release)
	return err
}
