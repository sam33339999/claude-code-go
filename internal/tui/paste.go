package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/tunsuy/glaude/pkg/types"
)

// pasteKind distinguishes a stored paste payload.
type pasteKind int

const (
	pasteText pasteKind = iota
	pasteImage
)

// Pasted text collapses when it exceeds either limit. Counts are taken after
// newline normalization: runes, not bytes, and a line is a segment split by '\n'.
const (
	pasteMaxRunes = 800
	pasteMaxLines = 10
)

type pastedItem struct {
	kind   pasteKind
	marker string
	text   string
	image  *types.ImageSource
}

// pasteStore keeps session-scoped pastes. IDs start at 1 and are not reused.
type pasteStore struct {
	items  map[int]*pastedItem
	nextID int
}

// markerRe matches a well-formed paste marker. isMarker still requires the
// token to equal the marker stored for that id; a regex hit alone is not enough.
var markerRe = regexp.MustCompile(`\[(?:Pasted text #\d+ (?:\+\d+ lines|\d+ chars)|Image #\d+)\]`)

func newPasteStore() *pasteStore {
	return &pasteStore{
		items:  make(map[int]*pastedItem),
		nextID: 1,
	}
}

// normalizeNewlines turns CRLF and lone CR into LF.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// shouldCollapse reports whether pasted text is over the rune or line threshold.
func shouldCollapse(text string) bool {
	text = normalizeNewlines(text)
	if utf8.RuneCountInString(text) > pasteMaxRunes {
		return true
	}
	return strings.Count(text, "\n")+1 > pasteMaxLines
}

// addText stores normalized text and returns its marker. The caller has already
// decided the text is over the collapse threshold.
func (s *pasteStore) addText(text string) string {
	text = normalizeNewlines(text)
	id := s.nextID
	s.nextID++
	newlines := strings.Count(text, "\n")
	var marker string
	if newlines > 0 {
		marker = fmt.Sprintf("[Pasted text #%d +%d lines]", id, newlines)
	} else {
		marker = fmt.Sprintf("[Pasted text #%d %d chars]", id, utf8.RuneCountInString(text))
	}
	s.items[id] = &pastedItem{kind: pasteText, marker: marker, text: text}
	return marker
}

// addImage stores an image source and returns its marker. Text and images share one ID sequence.
func (s *pasteStore) addImage(src *types.ImageSource) string {
	id := s.nextID
	s.nextID++
	marker := fmt.Sprintf("[Image #%d]", id)
	s.items[id] = &pastedItem{kind: pasteImage, marker: marker, image: src}
	return marker
}

// isMarker reports whether tok is exactly the marker stored for its id.
func (s *pasteStore) isMarker(tok string) bool {
	if markerRe.FindString(tok) != tok {
		return false
	}
	item := s.items[markerID(tok)]
	return item != nil && item.marker == tok
}

// markerID reads the paste id after '#'. It returns 0 when tok has no id.
func markerID(tok string) int {
	i := strings.IndexByte(tok, '#')
	if i < 0 || i+1 >= len(tok) {
		return 0
	}
	n := 0
	for _, r := range tok[i+1:] {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
