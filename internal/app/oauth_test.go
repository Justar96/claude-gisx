package app

import (
	"encoding/json"
	"testing"
	"time"
)

// Shape taken from a live /api/oauth/usage response.
const usageSample = `{
  "five_hour": {"utilization": 56.0, "resets_at": "2026-07-20T14:10:00.166596+00:00"},
  "seven_day_opus": null,
  "cinder_cove": null,
  "extra_usage": {"is_enabled": false, "monthly_limit": null, "used_credits": null},
  "limits": [
    {"kind": "session", "group": "session", "percent": 56, "resets_at": "2026-07-20T14:10:00+00:00", "scope": null},
    {"kind": "weekly_all", "group": "weekly", "percent": 65, "resets_at": "2026-07-22T09:00:00+00:00", "scope": null},
    {"kind": "weekly_scoped", "group": "weekly", "percent": 98, "resets_at": "2026-07-22T09:00:00+00:00",
     "scope": {"model": {"id": null, "display_name": "Fable"}, "surface": null}}
  ]
}`

func TestToUsageScopedLimits(t *testing.T) {
	var b oauthBody
	if err := json.Unmarshal([]byte(usageSample), &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	u := b.toUsage()
	if u == nil {
		t.Fatal("want usage, got nil")
	}
	if u.Extra != nil {
		t.Errorf("extra usage disabled with null credits, want nil, got %+v", u.Extra)
	}
	if len(u.Scoped) != 1 {
		t.Fatalf("want 1 scoped limit (unscoped kinds are the 5h/7d buckets), got %d", len(u.Scoped))
	}
	got := u.Scoped[0]
	want := scopedLimit{
		Name:     "Fable",
		Pct:      98,
		ResetsAt: time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC).Unix(),
	}
	if got != want {
		t.Errorf("scoped limit: got %+v, want %+v", got, want)
	}
}

func TestToUsageExtraCredits(t *testing.T) {
	const body = `{"extra_usage": {"is_enabled": true, "monthly_limit": 2000, "used_credits": 420}}`
	var b oauthBody
	if err := json.Unmarshal([]byte(body), &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	u := b.toUsage()
	if u == nil || u.Extra == nil {
		t.Fatal("want extra usage, got nil")
	}
	if u.Extra.Used != "4.20" || u.Extra.Limit != "20.00" {
		t.Errorf("got $%s/$%s, want $4.20/$20.00", u.Extra.Used, u.Extra.Limit)
	}
}

func TestToUsageEmpty(t *testing.T) {
	var b oauthBody
	if err := json.Unmarshal([]byte(`{"five_hour": null, "limits": []}`), &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u := b.toUsage(); u != nil {
		t.Errorf("nothing to show, want nil, got %+v", u)
	}
}
