package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The payload reports cost and a context percentage but never the token split,
// and stats-cache.json lumps everything into one daily number — neither can
// say how much of a request was served from the prompt cache. The transcript
// Claude Code is already appending to can: every assistant entry carries the
// usage the API billed for that request.
type tokenTotals struct {
	Input  int64 `json:"input"` // fresh input, never the cached prefix
	Write  int64 `json:"write"` // cache_creation_input_tokens
	Read   int64 `json:"read"`  // cache_read_input_tokens
	Output int64 `json:"output"`
}

// A transcript runs to megabytes over a long session and the statusline
// redraws on every message, so it's read the way it's written: from where the
// last render stopped. The offset only ever advances past a line terminator,
// so a render that lands mid-append re-reads that partial line next time.
type tokenCache struct {
	Offset int64       `json:"offset"`
	LastID string      `json:"last_id"`
	Totals tokenTotals `json:"totals"`
}

// One entry per session, named for the session so concurrent sessions don't
// fight over a shared file.
func tokenCachePath(transcriptPath string) string {
	sid := strings.TrimSuffix(filepath.Base(transcriptPath), ".jsonl")
	return filepath.Join(lineCacheDir(), "statusline-tokens-"+sid+".json")
}

// Only the fields worth billing for — the rest of an entry is the message body,
// which is the bulk of the bytes and none of the answer.
type transcriptEntry struct {
	Message struct {
		ID    string `json:"id"`
		Usage *struct {
			Input  int64 `json:"input_tokens"`
			Write  int64 `json:"cache_creation_input_tokens"`
			Read   int64 `json:"cache_read_input_tokens"`
			Output int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func sessionTokens(transcriptPath string) tokenTotals {
	if transcriptPath == "" {
		return tokenTotals{}
	}
	st, err := os.Stat(transcriptPath)
	if err != nil {
		return tokenTotals{}
	}
	c := loadTokenCache(transcriptPath)
	// Shorter than we left it means this isn't the file we summed; the totals
	// carried over from it would be someone else's.
	if c.Offset > st.Size() {
		c = tokenCache{}
	}
	if c.Offset == st.Size() {
		return c.Totals
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return c.Totals
	}
	defer f.Close()
	if _, err := f.Seek(c.Offset, io.SeekStart); err != nil {
		return c.Totals
	}
	fresh := c.Offset == 0
	scanTranscript(f, &c)
	saveTokenCache(transcriptPath, &c, fresh)
	return c.Totals
}

// scanTranscript folds every complete line from the current position into c.
func scanTranscript(r io.Reader, c *tokenCache) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return // an unterminated tail is a half-written append; leave it
		}
		c.Offset += int64(len(line))
		var e transcriptEntry
		if json.Unmarshal([]byte(line), &e) != nil || e.Message.Usage == nil {
			continue
		}
		// One assistant turn can be written as several entries (thinking, then
		// text), each repeating the same usage. They're always consecutive, so
		// remembering the last id counted is enough to bill the turn once.
		if e.Message.ID != "" && e.Message.ID == c.LastID {
			continue
		}
		c.LastID = e.Message.ID
		u := e.Message.Usage
		c.Totals.Input += u.Input
		c.Totals.Write += u.Write
		c.Totals.Read += u.Read
		c.Totals.Output += u.Output
	}
}

func loadTokenCache(transcriptPath string) tokenCache {
	var c tokenCache
	raw, err := os.ReadFile(tokenCachePath(transcriptPath))
	if err != nil || json.Unmarshal(raw, &c) != nil {
		return tokenCache{}
	}
	return c
}

// fresh marks the first write for a session, which is the one moment worth
// paying a directory listing for: every later write is on the render path.
func saveTokenCache(transcriptPath string, c *tokenCache, fresh bool) {
	raw, err := json.Marshal(c)
	if err != nil {
		return
	}
	if fresh {
		pruneTokenCaches()
	}
	_ = writeFileAtomic(tokenCachePath(transcriptPath), raw)
}

// These are per-session and nothing ever deletes the session, so without a
// sweep the cache dir grows a small file per conversation forever.
const tokenCacheTTL = 14 * 24 * time.Hour

func pruneTokenCaches() {
	ents, err := os.ReadDir(lineCacheDir())
	if err != nil {
		return
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "statusline-tokens-") {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < tokenCacheTTL {
			continue
		}
		_ = os.Remove(filepath.Join(lineCacheDir(), e.Name()))
	}
}

// paintTokens renders "cache 96% · in 1.2M · out 34k": what share of this
// session's input came off the prompt cache, and the totals it's a share of.
// `in` is everything the API charged as input, cached or not, so it counts the
// same cached prefix once per request — which is exactly how it's billed.
func paintTokens(t tokenTotals) string {
	in := t.Input + t.Write + t.Read
	if in == 0 {
		return ""
	}
	hit := int(t.Read * 100 / in)
	return white + "cache" + reset + " " + cacheColor(hit) + fmt.Sprintf("%d%%", hit) + reset +
		sep + dimGray + "in" + reset + " " + white + fmtTokens(in) + reset +
		sep + dimGray + "out" + reset + " " + white + fmtTokens(t.Output) + reset
}
