package msgqueue

import "testing"

func TestQueuedCommand_OriginPreserved(t *testing.T) {
	q := NewMessageQueue()
	cmd := NewCommand("[btw] Q: x", ModePrompt, PriorityNext)
	cmd.Origin = OriginBTW
	q.Enqueue(cmd)
	got := q.GetByMaxPriority(PriorityNext, "")
	if len(got) != 1 || got[0].Origin != OriginBTW {
		t.Errorf("origin lost: %+v", got)
	}
}
