package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// questionTestModel builds a model with live question HIL channels.
func questionTestModel() (AppModel, chan types.QuestionRequest, chan types.QuestionResponse) {
	ask := make(chan types.QuestionRequest, 1)
	resp := make(chan types.QuestionResponse, 1)
	m := newTestModel()
	m.questionAskCh = ask
	m.questionRespCh = resp
	return m, ask, resp
}

func TestQuestionDialog_OpensAndRenders(t *testing.T) {
	m, _, _ := questionTestModel()
	updated, cmd := m.Update(QuestionRequestMsg{
		RequestID: "q1",
		Question:  "which database?",
		Options:   []string{"postgres", "sqlite"},
		RespFn:    func(string, bool) {},
	})
	m = updated.(AppModel)
	if m.activeDialog != dialogQuestion {
		t.Fatalf("expected dialogQuestion, got %v", m.activeDialog)
	}
	if cmd == nil {
		t.Error("expected listener re-arm Cmd")
	}
	view := m.View().Content
	for _, want := range []string{"which database?", "1. postgres", "2. sqlite", "Esc dismiss"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in question dialog view", want)
		}
	}
}

func TestQuestionDialog_NumberSelectAndSubmit(t *testing.T) {
	m, _, _ := questionTestModel()
	var gotAnswer string
	var gotDismissed bool
	updated, _ := m.Update(QuestionRequestMsg{
		RequestID: "q1", Question: "pick", Options: []string{"a", "b"},
		RespFn: func(answer string, dismissed bool) { gotAnswer, gotDismissed = answer, dismissed },
	})
	m = updated.(AppModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(AppModel)
	if m.questionReq.selected != 1 {
		t.Errorf("expected option 2 selected, got %d", m.questionReq.selected)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(AppModel)
	if gotAnswer != "b" || gotDismissed {
		t.Errorf("expected answer b via RespFn, got %q dismissed=%v", gotAnswer, gotDismissed)
	}
	if m.activeDialog != dialogNone || m.questionReq != nil {
		t.Error("dialog must close after submit")
	}
}

func TestQuestionDialog_FreeTextAndSubmit(t *testing.T) {
	m, _, _ := questionTestModel()
	var gotAnswer string
	updated, _ := m.Update(QuestionRequestMsg{
		RequestID: "q1", Question: "why?",
		RespFn: func(answer string, _ bool) { gotAnswer = answer },
	})
	m = updated.(AppModel)
	for _, r := range "because" {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(AppModel)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = updated.(AppModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(AppModel)
	if gotAnswer != "becaus" {
		t.Errorf("expected free-text answer with backspace, got %q", gotAnswer)
	}
}

func TestQuestionDialog_EscDismisses(t *testing.T) {
	m, _, _ := questionTestModel()
	var gotDismissed bool
	updated, _ := m.Update(QuestionRequestMsg{
		RequestID: "q1", Question: "q",
		RespFn: func(_ string, dismissed bool) { gotDismissed = dismissed },
	})
	m = updated.(AppModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(AppModel)
	if !gotDismissed {
		t.Error("Esc must dismiss via RespFn")
	}
	if m.activeDialog != dialogNone {
		t.Error("dialog must close after Esc")
	}
}

func TestQuestionListener_EndToEnd(t *testing.T) {
	m, ask, resp := questionTestModel()
	cmd := listenForQuestionRequest(m.questionAskCh, m.questionRespCh)
	ask <- types.QuestionRequest{ID: "q9", Question: "live?", Options: []string{"yes"}}
	msg := cmd()
	qmsg, ok := msg.(QuestionRequestMsg)
	if !ok {
		t.Fatalf("expected QuestionRequestMsg, got %T", msg)
	}
	if qmsg.Question != "live?" || qmsg.RequestID != "q9" {
		t.Errorf("unexpected msg: %+v", qmsg)
	}
	qmsg.RespFn("yes", false)
	got := <-resp
	if got.ID != "q9" || got.Answer != "yes" || got.Dismissed {
		t.Errorf("unexpected response through channel: %+v", got)
	}
}
