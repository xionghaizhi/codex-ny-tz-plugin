package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConcurrencyRetainsShortSpikeAndCountsRequestOnce(t *testing.T) {
	meter := newConcurrencyMeter()
	start := time.Unix(1800000000, 0)
	meter.change("first", false, start)
	meter.change("first", false, start)
	meter.change("second", false, start)
	meter.change("second", true, start.Add(time.Second))
	meter.change("second", true, start.Add(time.Second))
	meter.change("first", true, start.Add(6*time.Second))
	stats := meter.snapshot(start.Add(11 * time.Second))
	if stats.Active != 0 || stats.Peak != 2 || stats.PeakAt != start.Unix() {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	want := []int{2, 1, 0}
	if len(stats.Points) != len(want) {
		t.Fatalf("unexpected points: %+v", stats.Points)
	}
	for i, peak := range want {
		if stats.Points[i].Peak != peak {
			t.Fatalf("point %d: got %d, want %d", i, stats.Points[i].Peak, peak)
		}
	}
	after := meter.snapshot(start.Add(time.Hour))
	if len(after.Points) != historySize || after.Points[0].Peak != 0 || after.Peak != 2 {
		t.Fatalf("old trend should expire without losing peak: %+v", after)
	}
}

func TestConcurrencyIgnoresLateAdmissionAfterCancellation(t *testing.T) {
	meter := newConcurrencyMeter()
	now := time.Unix(1800000000, 0)
	meter.change("cancelled", true, now)
	meter.change("cancelled", false, now.Add(time.Second))
	stats := meter.snapshot(now.Add(time.Second))
	if stats.Active != 0 || stats.Peak != 0 {
		t.Fatalf("late admission changed statistics: %+v", stats)
	}
}

func TestConcurrencyLifecycleDispatch(t *testing.T) {
	defer requestMeter.Store(nil)
	for _, callback := range []struct{ method, payload string }{
		{"plugin.register", `{"schema_version":2}`},
		{"request.intercept_before", `{"RequestID":"admitted"}`},
		{"request.intercept_after", `{"RequestID":"admitted"}`},
		{"request.complete", `{"RequestID":"admitted"}`},
	} {
		if _, err := handleMethod(callback.method, []byte(callback.payload)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := handleMethod("management.handle", []byte(`{"Method":"GET","Path":"/v0/management/plugins/codex-ny-timezone/concurrency"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Result concurrencyResourceResponse }
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	var stats concurrencyStats
	if err := json.Unmarshal(result.Result.Body, &stats); err != nil || result.Result.StatusCode != 200 || stats.Active != 0 || stats.Peak != 1 {
		t.Fatalf("stats status=%d body=%s, decode=%v", result.Result.StatusCode, result.Result.Body, err)
	}
}

func TestConcurrencyRetainsConfigReloadButResetsNewRegistration(t *testing.T) {
	defer requestMeter.Store(nil)
	config := []byte(`{"schema_version":2}`)
	for _, method := range []string{"plugin.register", "plugin.reconfigure", "plugin.register"} {
		previous := requestMeter.Load()
		if _, err := handleMethod(method, config); err != nil {
			t.Fatal(err)
		}
		if preserved := previous == requestMeter.Load(); preserved != (method == "plugin.reconfigure") {
			t.Fatalf("%s preserved prior meter: %v", method, preserved)
		}
	}
}
