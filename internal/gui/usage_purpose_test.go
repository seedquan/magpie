package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/usage"
)

func TestUsagePurposeRoutes(t *testing.T) {
	sandboxHome(t)
	start := usage.Today.Since(holdUsageClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))).Add(time.Second)
	for i, kind := range []string{"thread_title", "thread_title_reconsideration", "review", "", "future_kind", "title_generation"} {
		usage.Append(usage.Record{Time: start.Add(time.Duration(i) * time.Second), Agent: "codex", Provider: "relay", Model: "m", Kind: kind, Input: 10, Output: 2, Status: 200})
	}
	mux := http.NewServeMux()
	usageRoutes(mux, folderOnly{})
	q := url.Values{"period": {"today"}, "purpose": {"kind:thread_title"}, "limit": {"1"}, "offset": {"1"}, "agent": {"codex"}}
	get := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path+"?"+q.Encode(), nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w
	}
	var page ledgerJSON
	if err := json.Unmarshal(get("GET", "/api/usage/requests").Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || page.Calls != 3 || page.Input != 30 || page.Offset != 1 || len(page.Rows) != 1 || page.Rows[0].Kind != "thread_title_reconsideration" || len(page.Purposes) != 4 {
		t.Fatalf("purpose page: %+v", page)
	}
	var chartCalls int
	for _, p := range page.Series {
		chartCalls += p.Calls
	}
	if chartCalls != 3 {
		t.Fatalf("chart ignored purpose: %d", chartCalls)
	}

}
