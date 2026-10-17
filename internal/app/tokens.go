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

// Token caches and context maps are per-session and nothing ever deletes
// the session, so without a sweep the cache dir grows a small file per
// conversation forever.
const tokenCacheTTL = 14 * 24 * time.Hour

func pruneTokenCaches() {
	sweepStaleTemps(lineCacheDir())
	sweepStaleTemps(cacheDir())
	ents, err := os.ReadDir(lineCacheDir())
	if err != nil {
		return
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !(strings.HasPrefix(name, "statusline-tokens-") || strings.HasPrefix(name, "statusline-context-")) {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < tokenCacheTTL {
			continue
		}
		_ = os.Remove(filepath.Join(lineCacheDir(), name))
	}
}

// paintTokens renders the session's prompt cache in the terms Anthropic's
// docs use, state first and token counts after:
//
//	cache warm 52m of 1h left · idle compact in 46m · hit 93% · 2 misses · in 1.5M (1.4M read, 80k write, 2.0k uncached) · out 15.1k
//
// warm/cold, the TTL, the hit ratio and misses come from the payload's
// prompt_cache (Claude Code v2.1.251+, the figures /usage shows). Older
// versions get the hit ratio from the transcript sum and no state. `in` is
// total input — cache reads + cache writes + uncached input — so it counts
// the cached prefix once per request, exactly as it's billed. idleAt is when
// Claude Code compacts the session if nobody types (unix seconds, 0 for never).
func paintTokens(t tokenTotals, pc *promptCache, now time.Time, idleAt int64) string {
	in := t.Input + t.Write + t.Read
	hit := -1
	switch {
	case pc != nil && pc.HitRatio != nil:
		hit = int(*pc.HitRatio * 100)
	case in > 0:
		hit = int(t.Read * 100 / in)
	}

	var parts []string
	label := white + "cache" + reset + " "
	if state := cacheState(pc, now); state != "" {
		parts = append(parts, label+state)
		label = ""
	}
	if left := idleAt - now.Unix(); idleAt > 0 && left > 0 {
		parts = append(parts, magenta+"idle compact in "+fmtDuration(left*1000)+reset)
	}
	if hit >= 0 {
		h := label + dimGray + "hit" + reset + " " + cacheColor(hit) + fmt.Sprintf("%d%%", hit) + reset
		if pc != nil && pc.Misses > 0 {
			h += " " + yellow + fmt.Sprintf("%d %s", pc.Misses, plural(pc.Misses, "miss", "misses")) + reset
		}
		parts = append(parts, h)
	}
	if in > 0 {
		parts = append(parts,
			dimGray+"in"+reset+" "+white+fmtTokens(in)+reset+" "+
				dimGray+"("+fmtTokens(t.Read)+" read, "+fmtTokens(t.Write)+" write, "+
				fmtTokens(t.Input)+" uncached)"+reset,
			dimGray+"out"+reset+" "+white+fmtTokens(t.Output)+reset)
	}
	return strings.Join(parts, sep)
}

// A cache read refreshes the entry for free, so the last minute is the time
// to send the next message, and an expired prefix this large is cheaper to
// /compact than to write back at the cache-write rate.
const (
	cacheWarnSec       = 60
	defaultRecacheWarn = 100_000
)

// cacheState is "warm 4m of 1h left", "cold recache 412.0k /compact first", or
// "off" when no response this session reported cache tokens.
func cacheState(pc *promptCache, now time.Time) string {
	switch {
	case pc == nil:
		return ""
	case !pc.CachingObserved:
		return dimGray + "off" + reset
	case pc.Warm:
		left := pc.ExpiresAt - now.Unix()
		if pc.ExpiresAt <= 0 || left <= 0 {
			s := green + "warm" + reset
			if pc.TTL != "" {
				s += " " + dimGray + pc.TTL + reset
			}
			return s
		}
		col := cacheLifeColor(left, pc.TTL)
		s := col + "warm " + fmtDuration(left*1000) + reset + dimGray
		if pc.TTL != "" {
			s += " of " + pc.TTL
		}
		return s + " left" + reset
	default:
		s := blue + "cold" + reset
		if pc.RecacheIfCold != nil && *pc.RecacheIfCold > 0 {
			s += " " + dimGray + "recache " + fmtTokens(*pc.RecacheIfCold) + reset
			if *pc.RecacheIfCold >= int64(envInt("CLAUDE_GISX_RECACHE_WARN", defaultRecacheWarn)) {
				s += " " + yellow + "/compact first" + reset
			}
		}
		return s
	}
}

// cacheLifeColor grades the time a warm cache has left: green while there's
// plenty, yellow from 40% of its lifetime down, red in the last minute.
func cacheLifeColor(leftSec int64, ttl string) string {
	life := map[string]int64{"5m": 300, "1h": 3600}[ttl]
	switch {
	case leftSec <= cacheWarnSec:
		return red
	case life > 0 && leftSec*10 <= life*4:
		return yellow
	default:
		return green
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
