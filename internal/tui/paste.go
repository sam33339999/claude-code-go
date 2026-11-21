package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
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

// expansion is one paste-marker pass. Text markers are replaced by their
// stored body. Image markers stay in the text and are collected once each,
// in first-seen order. The replacement is not scanned again.
type expansion struct {
	Text   string
	Images []types.ContentBlock
}

// expand rewrites paste markers in a single ReplaceAllStringFunc pass.
// A stored body that itself contains marker-shaped text stays literal.
func (s *pasteStore) expand(text string) expansion {
	if s == nil {
		return expansion{Text: text}
	}
	seen := make(map[int]bool)
	var images []types.ContentBlock
	out := markerRe.ReplaceAllStringFunc(text, func(tok string) string {
		if !s.isMarker(tok) {
			return tok
		}
		item := s.items[markerID(tok)]
		switch item.kind {
		case pasteText:
			return item.text
		case pasteImage:
			id := markerID(tok)
			if !seen[id] {
				seen[id] = true
				images = append(images, types.ContentBlock{
					Type:   types.ContentTypeImage,
					Source: item.image,
				})
			}
			return tok
		default:
			return tok
		}
	})
	return expansion{Text: out, Images: images}
}

// expandTextOnly expands text markers and reports whether any image marker
// was dropped instead of attached.
func (s *pasteStore) expandTextOnly(text string) (string, bool) {
	exp := s.expand(text)
	return exp.Text, len(exp.Images) > 0
}

// contentWithImages builds [images…, text]. No images means one text block.
func contentWithImages(images []types.ContentBlock, text string) []types.ContentBlock {
	blocks := make([]types.ContentBlock, 0, len(images)+1)
	blocks = append(blocks, images...)
	blocks = append(blocks, types.ContentBlock{Type: types.ContentTypeText, Text: strPtr(text)})
	return blocks
}

// isMarker reports whether tok is exactly the marker stored for its id.
func (s *pasteStore) isMarker(tok string) bool {
	if markerRe.FindString(tok) != tok {
		return false
	}
	item := s.items[markerID(tok)]
	return item != nil && item.marker == tok
}

// deleteDir selects which cursor relationship removes a paste marker.
type deleteDir int

const (
	delBackward deleteDir = iota
	delForward
	delWordBackward
)

// tokenSpan returns the half-open rune range of one markerRe hit on line that
// isToken accepts and that dir would delete at col. A regex hit that isToken
// rejects is not a token.
func tokenSpan(line []rune, col int, dir deleteDir, isToken func(string) bool) (start, end int, ok bool) {
	if isToken == nil {
		return 0, 0, false
	}
	if col < 0 {
		col = 0
	} else if col > len(line) {
		col = len(line)
	}
	probe := col
	if dir == delWordBackward {
		probe = skipSpaceLeft(line, col)
	}
	text := string(line)
	for _, loc := range markerRe.FindAllStringIndex(text, -1) {
		s := utf8.RuneCountInString(text[:loc[0]])
		e := s + utf8.RuneCountInString(text[loc[0]:loc[1]])
		if e > len(line) || !isToken(string(line[s:e])) {
			continue
		}
		if spanMatches(dir, s, e, col, probe) {
			return s, e, true
		}
	}
	return 0, 0, false
}

func spanMatches(dir deleteDir, s, e, col, probe int) bool {
	switch dir {
	case delBackward:
		return s < col && col <= e
	case delForward:
		return s <= col && col < e
	case delWordBackward:
		return s < probe && probe <= e
	default:
		return false
	}
}

// skipSpaceLeft walks left over whitespace immediately before col.
func skipSpaceLeft(line []rune, col int) int {
	for col > 0 && unicode.IsSpace(line[col-1]) {
		col--
	}
	return col
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
