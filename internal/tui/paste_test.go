package tui

import (
	"strings"
	"testing"

	"github.com/tunsuy/glaude/pkg/types"
)

func TestShouldCollapse(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{strings.Repeat("a", 800), false},
		{strings.Repeat("a", 801), true},
		{strings.Repeat("中", 801), true},          // rune 計數
		{strings.Repeat("x\n", 9) + "x", false},   // 10 行
		{strings.Repeat("x\n", 10) + "x", true},   // 11 行
		{strings.Repeat("x\r\n", 10) + "x", true}, // CRLF 正規化
	}
	for _, c := range cases {
		if got := shouldCollapse(c.in); got != c.want {
			t.Errorf("shouldCollapse(%d runes) = %v, want %v", len([]rune(c.in)), got, c.want)
		}
	}
}

func TestPasteStore_Markers(t *testing.T) {
	s := newPasteStore()
	if m := s.addText(strings.Repeat("l\n", 312) + "end"); m != "[Pasted text #1 +312 lines]" {
		t.Errorf("got %q", m)
	}
	if m := s.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="}); m != "[Image #2]" {
		t.Errorf("got %q", m)
	}
	if m := s.addText(strings.Repeat("a", 1234)); m != "[Pasted text #3 1234 chars]" {
		t.Errorf("got %q", m)
	}
	for tok, want := range map[string]bool{
		"[Pasted text #1 +312 lines]": true,
		"[Pasted text #1 +311 lines]": false, // 行數不符
		"[Image #2]":                  true,
		"[Image #1]":                  false, // #1 是文字
		"[Image #9]":                  false, // 不存在
	} {
		if got := s.isMarker(tok); got != want {
			t.Errorf("isMarker(%q) = %v", tok, got)
		}
	}
}
