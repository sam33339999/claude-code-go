package acpclient

import (
	"context"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// client implements acp.Client for one session. glaude advertises no fs or
// terminal capabilities, so agents use their own tools; those methods reject.
type client struct{ s *session }

var _ acp.Client = (*client)(nil)

func (c *client) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	s := c.s
	u := n.Update
	switch {
	case u.AgentMessageChunk != nil:
		if t := blockText(u.AgentMessageChunk.Content); t != "" {
			s.mu.Lock()
			s.reply.WriteString(t)
			s.mu.Unlock()
			s.emit(Event{Kind: EventText, Text: t})
		}
	case u.AgentThoughtChunk != nil:
		if t := blockText(u.AgentThoughtChunk.Content); t != "" {
			s.emit(Event{Kind: EventThought, Text: t})
		}
	case u.ToolCall != nil:
		tc := u.ToolCall
		s.mu.Lock()
		if s.titles == nil {
			s.titles = make(map[acp.ToolCallId]string)
		}
		s.titles[tc.ToolCallId] = tc.Title
		s.mu.Unlock()
		s.emit(Event{Kind: EventToolCall, ToolCallID: string(tc.ToolCallId), Title: tc.Title, Status: string(tc.Status)})
	case u.ToolCallUpdate != nil:
		tu := u.ToolCallUpdate
		s.mu.Lock()
		title := s.titles[tu.ToolCallId]
		if t, ok := tu.Title.Value(); ok && t != "" {
			title = t
			if s.titles != nil {
				s.titles[tu.ToolCallId] = t
			}
		}
		s.mu.Unlock()
		status, _ := tu.Status.Value()
		s.emit(Event{Kind: EventToolUpdate, ToolCallID: string(tu.ToolCallId), Title: title, Status: string(status)})
	case u.Plan != nil:
		lines := make([]string, 0, len(u.Plan.Entries))
		for _, e := range u.Plan.Entries {
			lines = append(lines, "["+string(e.Status)+"] "+e.Content)
		}
		s.emit(Event{Kind: EventPlan, Text: strings.Join(lines, "\n")})
	}
	return nil
}

func (c *client) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	s := c.s
	cancelled := acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{Outcome: "cancelled"}}}
	q := MapToolCall(s.spec.Name, p.ToolCall)
	allow := false
	if s.perm != nil {
		pctx := s.currentCtx(ctx)
		ok, err := s.perm(pctx, q)
		if err != nil {
			s.logf("acp %s: permission check for %s failed: %v", s.spec.Name, q.ToolName, err)
		}
		if pctx.Err() != nil {
			return cancelled, nil
		}
		allow = ok && err == nil
	}
	id, ok := PickOption(p.Options, allow)
	if !ok {
		return cancelled, nil
	}
	if !allow {
		s.emit(Event{Kind: EventNotice, Text: "denied: " + q.ToolName + " " + q.Title})
	}
	return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: id, Outcome: "selected"}}}, nil
}

func (c *client) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, acp.NewMethodNotFound(acp.ClientMethodFsReadTextFile)
}

func (c *client) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, acp.NewMethodNotFound(acp.ClientMethodFsWriteTextFile)
}

func (c *client) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalCreate)
}

func (c *client) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalKill)
}

func (c *client) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalOutput)
}

func (c *client) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalRelease)
}

func (c *client) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalWaitForExit)
}

func (c *client) CreateElicitation(context.Context, acp.CreateElicitationRequest) (acp.CreateElicitationResponse, error) {
	return acp.CreateElicitationResponse{Decline: &acp.CreateElicitationDecline{Action: "decline"}}, nil
}

func (c *client) CompleteElicitation(context.Context, acp.CompleteElicitationNotification) error {
	return nil
}

func blockText(b acp.ContentBlock) string {
	if b.Text != nil {
		return b.Text.Text
	}
	return ""
}
