package coordinator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tunsuy/glaude/internal/coordinator"
)

// ─────────────────────────────────────────────────────────────────────────────
// Thread lifecycle: stop preservation, resume, message stranding
// ─────────────────────────────────────────────────────────────────────────────

// TestStopAgent_StatusPreservedAfterLoopExit guards the fix for the historical
// TestAdapterStopAgent flake: StopAgent's "killed" transition must survive the
// run loop's exit (which used to clobber it with "completed").
func TestStopAgent_StatusPreservedAfterLoopExit(t *testing.T) {
	c := newCoordinator(blockedAgent("late-result"))
	id, err := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work", Description: "stop-preserve"})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.StopAgent(context.Background(), id); err != nil {
		t.Fatalf("StopAgent: %v", err)
	}

	// Give the loop time to observe cancellation and run its exit path.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		status, err := c.GetAgentStatus(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if status != coordinator.AgentStatusStopped {
			t.Fatalf("status after stop = %q, want killed (loop exit clobbered it)", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSendMessage_StoppedAgentResumes verifies a stopped thread can be
// reopened with a follow-up message.
func TestSendMessage_StoppedAgentResumes(t *testing.T) {
	var mu sync.Mutex
	runs := 0
	fn := func(ctx context.Context, _ coordinator.AgentID, _ coordinator.SpawnRequest, inbox <-chan string) (string, error) {
		mu.Lock()
		runs++
		n := runs
		mu.Unlock()
		if n == 1 {
			<-ctx.Done()
			return "", ctx.Err()
		}
		select {
		case m := <-inbox:
			return "resumed:" + m, nil
		case <-time.After(2 * time.Second):
			return "resumed:no-message", nil
		}
	}
	c := newCoordinator(fn)
	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work", Description: "stop-resume"})

	if err := c.StopAgent(context.Background(), id); err != nil {
		t.Fatalf("StopAgent: %v", err)
	}
	// Wait for the killed status to settle.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := c.GetAgentStatus(context.Background(), id)
		if s == coordinator.AgentStatusStopped {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := c.SendMessage(context.Background(), id, "pick it back up"); err != nil {
		t.Fatalf("SendMessage to stopped agent: %v", err)
	}
	ch, _ := c.Subscribe(id)
	select {
	case notif := <-ch:
		if notif.Result != "resumed:pick it back up" {
			t.Fatalf("resumed result = %q, want resumed:pick it back up", notif.Result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for resumed agent")
	}
}

// TestSendMessage_NoStrandedMessages verifies the atomic enqueue/exit-check
// protocol: messages sent concurrently around an agent's completion are all
// consumed (either by a continuation re-run or by a resume).
func TestSendMessage_NoStrandedMessages(t *testing.T) {
	var mu sync.Mutex
	consumed := 0
	fn := func(_ context.Context, _ coordinator.AgentID, _ coordinator.SpawnRequest, inbox <-chan string) (string, error) {
		// Drain with a short idle window so late deliveries within a run are
		// still consumed.
		idle := time.NewTimer(50 * time.Millisecond)
		defer idle.Stop()
		for {
			select {
			case <-inbox:
				mu.Lock()
				consumed++
				mu.Unlock()
				if !idle.Stop() {
					<-idle.C
				}
				idle.Reset(50 * time.Millisecond)
			case <-idle.C:
				return "", nil
			}
		}
	}
	c := newCoordinator(fn)
	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work", Description: "no-stranding"})

	// Wait for the initial run to finish.
	ch, _ := c.Subscribe(id)
	<-ch

	const n = 8
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.SendMessage(context.Background(), id, "msg"); err != nil {
				t.Errorf("SendMessage: %v", err)
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := consumed
		mu.Unlock()
		if got == n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("consumed = %d, want %d (messages stranded)", consumed, n)
}

// TestSpawnAgent_SurvivesParentContextCancel guards the live-fire fix: the
// spawn ctx is the parent query's tool-call context, cancelled when the
// parent turn ends. Background threads must outlive it (Projects semantics).
func TestSpawnAgent_SurvivesParentContextCancel(t *testing.T) {
	release := make(chan struct{})
	fn := func(ctx context.Context, _ coordinator.AgentID, _ coordinator.SpawnRequest, _ <-chan string) (string, error) {
		select {
		case <-release:
			return "survived", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	c := newCoordinator(fn)
	spawnCtx, cancel := context.WithCancel(context.Background())
	id, err := c.SpawnAgent(spawnCtx, coordinator.SpawnRequest{Prompt: "work", Description: "outlive-parent"})
	if err != nil {
		t.Fatal(err)
	}

	// Parent turn ends.
	cancel()
	time.Sleep(50 * time.Millisecond)

	status, err := c.GetAgentStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if status != coordinator.AgentStatusRunning {
		t.Fatalf("agent status after parent ctx cancel = %q, want running (lifetime must be decoupled)", status)
	}

	close(release)
	ch, _ := c.Subscribe(id)
	select {
	case notif := <-ch:
		if notif.Result != "survived" || notif.Status != coordinator.AgentStatusCompleted {
			t.Fatalf("notif = %+v, want completed/survived", notif)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for agent completion")
	}
}

// TestSendMessage_ResumeAfterParentContextCancel verifies resume works even
// when the original spawn context is long cancelled (entry.parentCtx must be
// the decoupled context, not the caller's).
func TestSendMessage_ResumeAfterParentContextCancel(t *testing.T) {
	var mu sync.Mutex
	runs := 0
	fn := func(_ context.Context, _ coordinator.AgentID, _ coordinator.SpawnRequest, inbox <-chan string) (string, error) {
		mu.Lock()
		runs++
		n := runs
		mu.Unlock()
		if n == 1 {
			return "first", nil
		}
		select {
		case m := <-inbox:
			return "resumed:" + m, nil
		case <-time.After(2 * time.Second):
			return "resumed:no-message", nil
		}
	}
	c := newCoordinator(fn)
	spawnCtx, cancel := context.WithCancel(context.Background())
	id, _ := c.SpawnAgent(spawnCtx, coordinator.SpawnRequest{Prompt: "work", Description: "resume-after-cancel"})
	ch, _ := c.Subscribe(id)
	<-ch
	cancel() // parent context gone before the follow-up

	if err := c.SendMessage(context.Background(), id, "follow-up"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	ch2, _ := c.Subscribe(id)
	select {
	case notif := <-ch2:
		if notif.Result != "resumed:follow-up" {
			t.Fatalf("resumed result = %q, want resumed:follow-up", notif.Result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for resumed agent")
	}
}
