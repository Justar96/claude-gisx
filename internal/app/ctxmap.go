package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The status line's payload says how full the context window is but never
// what fills it. /context's breakdown reaches mods alone, so the gisx mod
// (mods/gisx) pipes it to `claude-gisx ingest` after each turn. The next
// render colors line 1's context bar by category and keys the colors on the
// line beneath it, biggest first:
//
//	Opus 5.5 · ██████▌░░░ 66%/374k · …
//	ctx messages 112.0k · tools 31.0k · MCP 18.0k · system 9.0k · free 197.0k
//
// Without the mod there's no file: the bar keeps its single threshold color
// and the key is left out. Nothing is guessed.
type ctxBreakdown struct {
	V                    int           `json:"v"`
	SessionID            string        `json:"session_id"`
	MaxTokens            int64         `json:"max_tokens"`
	TotalTokens          int64         `json:"total_tokens"`
	AutoCompactThreshold *int64        `json:"auto_compact_threshold"`
	Categories           []ctxCategory `json:"categories"`
}

type ctxCategory struct {
	Name   string `json:"name"`
	Tokens int64  `json:"tokens"`
	Kind   string `json:"kind"`  // used, free, buffer or deferred (outside the window)
	Color  string `json:"color"` // the theme key /context draws the row in
}

// Session ids name the file, so only ids that can't climb out of the dir.
var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func ctxMapPath(sid string) string {
	return filepath.Join(lineCacheDir(), "statusline-context-"+sid+".json")
}

// ingestCmd is the mod's way in: one breakdown on stdin, kept beside the
// session for the next render. Written atomically, as a render may read it
// mid-write.
func ingestCmd(r io.Reader) int {
	raw, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err == nil {
		err = storeCtxMap(raw)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest: %v\n", err)
		return 1
	}
	return 0
}

func storeCtxMap(raw []byte) error {
	var b ctxBreakdown
	if err := json.Unmarshal(raw, &b); err != nil {
		return err
	}
	switch {
	case b.V != 1:
		return fmt.Errorf("unknown version %d", b.V)
	case !sessionIDRe.MatchString(b.SessionID):
		return errors.New("bad session_id")
	case b.MaxTokens <= 0:
		return errors.New("no max_tokens")
	}
	out, err := json.Marshal(b)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(lineCacheDir(), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(ctxMapPath(b.SessionID), out)
}

func loadCtxMap(sid string) *ctxBreakdown {
	if !sessionIDRe.MatchString(sid) {
		return nil
	}
	raw, err := os.ReadFile(ctxMapPath(sid))
	if err != nil {
		return nil
	}
	var b ctxBreakdown
	if json.Unmarshal(raw, &b) != nil || b.MaxTokens <= 0 {
		return nil
	}
	return &b
}

// The rows /context lists, by the short names the line has room for.
var ctxLabels = map[string]string{
	"System prompt":           "system",
	"System tools":            "tools",
	"MCP tools":               "MCP",
	"MCP server instructions": "MCP info",
	"Custom agents":           "agents",
	"Memory files":            "memory",
	"Skills":                  "skills",
	"Messages":                "messages",
}

func ctxLabel(name string) string {
	if l, ok := ctxLabels[name]; ok {
		return l
	}
	return strings.ToLower(name)
}

// ctxColor maps the theme key /context paints a row in onto the status line's
// palette. The two grays /context uses for the system rows are told apart
// here, since a bar can't lean on a legend beside it.
func ctxColor(themeKey string, i int) string {
	if reset == "" {
		return ""
	}
	switch themeKey {
	case "promptBorder":
		return blue
	case "inactive":
		return rgb(140, 150, 170)
	case "cyan_FOR_SUBAGENTS_ONLY":
		return cyan
	case "green_FOR_SUBAGENTS_ONLY":
		return green
	case "permission":
		return rgb(177, 185, 249)
	case "claude":
		return rgb(215, 119, 87)
	case "warning":
		return yellow
	case "purple_FOR_SUBAGENTS_ONLY":
		return magenta
	}
	return []string{green, orange, white}[i%3]
}

// ctxSeg is one category in use, colored the same on the bar and its key.
type ctxSeg struct {
	label  string
	tokens int64
	color  string
}

// ctxSegments are the categories in use in /context's own order — the fixed
// overhead first, the conversation last — so the bar grows at its right end
// as the session goes on.
func ctxSegments(b *ctxBreakdown) []ctxSeg {
	if b == nil {
		return nil
	}
	var segs []ctxSeg
	for _, c := range b.Categories {
		if c.Kind == "used" && c.Tokens > 0 {
			segs = append(segs, ctxSeg{ctxLabel(c.Name), c.Tokens, ctxColor(c.Color, len(segs))})
		}
	}
	return segs
}

// paintCtxKey names the bar's colors, biggest first, then what's left of the
// window: the free space and the auto-compact buffer.
func paintCtxKey(b *ctxBreakdown) string {
	segs := ctxSegments(b)
	if len(segs) == 0 {
		return ""
	}
	sort.SliceStable(segs, func(i, j int) bool { return segs[i].tokens > segs[j].tokens })
	var free, buffer int64
	for _, c := range b.Categories {
		switch c.Kind {
		case "free":
			free += c.Tokens
		case "buffer":
			buffer += c.Tokens
		}
	}
	var k strings.Builder
	k.WriteString(white + "ctx" + reset)
	for i, s := range segs {
		if i > 0 {
			k.WriteString(sep)
		} else {
			k.WriteString(" ")
		}
		k.WriteString(s.color + s.label + reset + " " + white + fmtTokens(s.tokens) + reset)
	}
	if free > 0 {
		k.WriteString(sep + dimGray + "free " + fmtTokens(free) + reset)
	}
	if buffer > 0 {
		k.WriteString(sep + dimGray + "buffer " + fmtTokens(buffer) + reset)
	}
	return k.String()
}

// apportion splits width cells across sizes by the largest remainder,
// giving each of the first minOne non-empty sizes at least one cell.
func apportion(sizes []int64, width, minOne int) []int {
	cells := make([]int, len(sizes))
	var total int64
	for _, s := range sizes {
		total += s
	}
	if total <= 0 {
		return cells
	}
	type rem struct {
		i int
		r int64
	}
	rems := make([]rem, len(sizes))
	given := 0
	for i, s := range sizes {
		cells[i] = int(s * int64(width) / total)
		rems[i] = rem{i, s * int64(width) % total}
		given += cells[i]
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r > rems[b].r })
	for k := 0; given < width; k = (k + 1) % len(rems) {
		cells[rems[k].i]++
		given++
	}
	// The floor of one cell is paid for by whichever segment has most to spare.
	for i := 0; i < minOne && i < len(sizes); i++ {
		if sizes[i] <= 0 || cells[i] > 0 {
			continue
		}
		donor := 0
		for j := range cells {
			if cells[j] > cells[donor] {
				donor = j
			}
		}
		if cells[donor] <= 1 {
			break
		}
		cells[donor]--
		cells[i]++
	}
	return cells
}
