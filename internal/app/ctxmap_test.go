package app

import (
	"strings"
	"testing"
)

const sampleIngest = `{"v":1,"session_id":"b1d2c3e4-0000-4000-8000-000000000001","max_tokens":387000,"total_tokens":121000,"auto_compact_threshold":374000,"categories":[
{"name":"System prompt","tokens":9000,"kind":"used","color":"promptBorder"},
{"name":"System tools","tokens":31000,"kind":"used","color":"inactive"},
{"name":"MCP tools","tokens":1200,"kind":"deferred","color":"cyan_FOR_SUBAGENTS_ONLY"},
{"name":"Messages","tokens":112000,"kind":"used","color":"purple_FOR_SUBAGENTS_ONLY"},
{"name":"Memory files","tokens":400,"kind":"used","color":"claude"},
{"name":"Free space","tokens":221600,"kind":"free","color":"promptBorder"},
{"name":"Autocompact buffer","tokens":13000,"kind":"buffer","color":"inactive"}]}`

func TestIngestRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	if code := ingestCmd(strings.NewReader(sampleIngest)); code != 0 {
		t.Fatalf("ingest exit %d", code)
	}
	b := loadCtxMap("b1d2c3e4-0000-4000-8000-000000000001")
	if b == nil || b.MaxTokens != 387_000 || len(b.Categories) != 7 || *b.AutoCompactThreshold != 374_000 {
		t.Fatalf("loaded %+v", b)
	}
	if loadCtxMap("someone-else") != nil {
		t.Error("another session's id should find nothing")
	}
}

func TestIngestRejects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for name, body := range map[string]string{
		"not json":       `{`,
		"newer version":  `{"v":2,"session_id":"abc","max_tokens":1}`,
		"path in the id": `{"v":1,"session_id":"../../etc/x","max_tokens":1}`,
		"no window":      `{"v":1,"session_id":"abc","max_tokens":0}`,
	} {
		if err := storeCtxMap([]byte(body)); err == nil {
			t.Errorf("%s: stored, want an error", name)
		}
	}
	if loadCtxMap("../x") != nil {
		t.Error("an id with a path in it should never be read")
	}
}

func TestPaintCtxKey(t *testing.T) {
	if paintCtxKey(nil) != "" {
		t.Error("no breakdown should draw nothing")
	}
	t.Setenv("HOME", t.TempDir())
	if err := storeCtxMap([]byte(sampleIngest)); err != nil {
		t.Fatal(err)
	}
	got := ansiRe.ReplaceAllString(paintCtxKey(loadCtxMap("b1d2c3e4-0000-4000-8000-000000000001")), "")
	// Biggest first; the deferred MCP row sits outside the window.
	want := "ctx messages 112.0k · tools 31.0k · system 9.0k · memory 400 · free 221.6k · buffer 13.0k"
	if got != want {
		t.Errorf("paintCtxKey:\n got %q\nwant %q", got, want)
	}
}

// The bar and its key must agree on which color is which category.
func TestCtxSegmentsColors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := storeCtxMap([]byte(sampleIngest)); err != nil {
		t.Fatal(err)
	}
	b := loadCtxMap("b1d2c3e4-0000-4000-8000-000000000001")
	segs := ctxSegments(b)
	var order []string
	for _, s := range segs {
		order = append(order, s.label)
	}
	// /context's order, not size: the conversation comes last.
	if got := strings.Join(order, ","); got != "system,tools,messages,memory" {
		t.Errorf("segments in %s, want /context's order", got)
	}
	key := paintCtxKey(b)
	for _, s := range segs {
		if !strings.Contains(key, s.color+s.label) {
			t.Errorf("the key doesn't paint %s in its bar color", s.label)
		}
	}
}

func TestApportion(t *testing.T) {
	cases := []struct {
		sizes  []int64
		minOne int
		want   []int
	}{
		{[]int64{50, 50}, 0, []int{12, 12}},
		{[]int64{1, 999}, 1, []int{1, 23}},
		{[]int64{0, 10}, 1, []int{0, 24}},
		{[]int64{0, 0}, 0, []int{0, 0}},
	}
	for _, c := range cases {
		got := apportion(c.sizes, 24, c.minOne)
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("apportion(%v) = %v, want %v", c.sizes, got, c.want)
				break
			}
		}
	}
}
