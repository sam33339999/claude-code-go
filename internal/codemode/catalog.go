package codemode

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// CatalogEntry is one tool's discoverable surface.
type CatalogEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// BuildCatalog snapshots the registry's tool surface for the sandbox catalog.
// Descriptions are trimmed to the first line (the full text stays available
// via describe). The snapshot is taken once per CodeMode call — the tool
// surface is session-pinned, so this is cache-stable.
func BuildCatalog(reg *tools.Registry) []CatalogEntry {
	if reg == nil {
		return nil
	}
	all := reg.All()
	entries := make([]CatalogEntry, 0, len(all))
	for _, t := range all {
		schema, _ := json.Marshal(t.InputSchema())
		entries = append(entries, CatalogEntry{
			Name:        t.Name(),
			Description: firstLine(t.Description(tools.Input("{}"), nil)),
			InputSchema: schema,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// Search ranks catalog entries against a natural-language query (keyword
// scoring: name tokens ×3, description ×1; same spirit as BF-12 ToolSearch).
// Returns up to 10 names, best first. Empty query returns all names.
func Search(entries []CatalogEntry, query string) []string {
	tokens := catalogTokens(query)
	if len(tokens) == 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name)
		}
		return names
	}
	type scored struct {
		name  string
		score int
	}
	var matches []scored
	for _, e := range entries {
		nameTokens := catalogTokens(e.Name)
		desc := strings.ToLower(e.Description)
		score := 0
		for _, q := range tokens {
			if containsToken(nameTokens, q) {
				score += 3
			} else if strings.Contains(desc, q) {
				score++
			}
		}
		if score > 0 {
			matches = append(matches, scored{name: e.Name, score: score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].name < matches[j].name
	})
	if len(matches) > 10 {
		matches = matches[:10]
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m.name)
	}
	return names
}

// Describe returns the full entry (including input schema) for one tool.
func Describe(entries []CatalogEntry, name string) (CatalogEntry, bool) {
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// CatalogText renders the compact up-front catalog for the tool description:
// one line per tool (name — first-line description). Full schemas are pulled
// on demand via codemode.describe inside the sandbox (progressive discovery).
func CatalogText(entries []CatalogEntry) string {
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString("- tools." + e.Name + "(input) — " + e.Description + "\n")
	}
	return sb.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}

// catalogTokens lowercases and splits on non-alphanumerics, splitting
// CamelCase at case boundaries first.
func catalogTokens(s string) []string {
	var spaced strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			spaced.WriteRune(' ')
		}
		spaced.WriteRune(r)
	}
	var tokens []string
	for _, f := range strings.FieldsFunc(strings.ToLower(spaced.String()), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if f != "" {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

func containsToken(tokens []string, tok string) bool {
	for _, t := range tokens {
		if t == tok || strings.HasPrefix(t, tok) {
			return true
		}
	}
	return false
}
