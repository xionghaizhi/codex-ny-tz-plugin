package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConcurrencyRetainsShortSpikeAndCountsRequestOnce(t *testing.T) {
	meter := newConcurrencyMeterAt("")
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
	if len(after.Points) != historySize || after.Points[0].Peak != 0 || after.Peak != 2 || after.HistoricalPeak != 2 {
		t.Fatalf("old trend should expire without losing peak: %+v", after)
	}
}

func TestConcurrencyIgnoresLateAdmissionAfterCancellation(t *testing.T) {
	meter := newConcurrencyMeterAt("")
	now := time.Unix(1800000000, 0)
	meter.change("cancelled", true, now)
	meter.change("cancelled", false, now.Add(time.Second))
	stats := meter.snapshot(now.Add(time.Second))
	if stats.Active != 0 || stats.Peak != 0 {
		t.Fatalf("late admission changed statistics: %+v", stats)
	}
}

func TestConcurrencyLifecycleDispatch(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("AppData", configHome)
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
	waitForHistory(t, requestMeter.Load())
	data, err := handleMethod("management.handle", []byte(`{"Method":"GET","Path":"/v0/management/plugins/codex-ny-timezone/concurrency"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Result concurrencyResourceResponse }
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	var stats concurrencyStats
	if err := json.Unmarshal(result.Result.Body, &stats); err != nil || result.Result.StatusCode != 200 ||
		stats.Active != 0 || stats.Peak != 1 || stats.HistoricalPeak != 1 || !stats.HistoricalPersisted {
		t.Fatalf("stats status=%d body=%s, decode=%v", result.Result.StatusCode, result.Result.Body, err)
	}
}

func TestConcurrencyRetainsConfigReloadButResetsNewRegistration(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("AppData", configHome)
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

func TestConcurrencyRegistrationRestoresPendingPeak(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("AppData", configHome)
	defer requestMeter.Store(nil)
	config := []byte(`{"schema_version":2}`)
	if _, err := handleMethod("plugin.register", config); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1800000000, 0)
	previous := requestMeter.Load()
	previous.change("first", false, start)
	previous.change("second", false, start)
	if _, err := handleMethod("plugin.register", config); err != nil {
		t.Fatal(err)
	}
	stats := requestMeter.Load().snapshot(start)
	if stats.Active != 0 || stats.Peak != 0 || stats.HistoricalPeak != 2 || !stats.HistoricalPersisted {
		t.Fatalf("new registration missed pending peak: %+v", stats)
	}
}

func TestConcurrencyLoadsHistoricalPeakAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	first := newConcurrencyMeterAt(path)
	start := time.Unix(1800000000, 0)
	first.change("one", false, start)
	first.change("two", false, start)
	first.change("one", true, start.Add(time.Second))
	first.change("two", true, start.Add(time.Second))
	waitForHistory(t, first)

	restarted := newConcurrencyMeterAt(path)
	stats := restarted.snapshot(start.Add(2 * time.Second))
	if stats.Peak != 0 || stats.HistoricalPeak != 2 || stats.HistoricalPeakAt != start.Unix() || !stats.HistoricalPersisted {
		t.Fatalf("historical stats were not restored: %+v", stats)
	}
	restarted.change("three", false, start.Add(3*time.Second))
	if stats := restarted.snapshot(start.Add(3 * time.Second)); stats.Peak != 1 || stats.HistoricalPeak != 2 || stats.HistoricalPeakAt != start.Unix() {
		t.Fatalf("historical peak changed unexpectedly: %+v", stats)
	}
	restarted.change("three", true, start.Add(4*time.Second))
	later := start.Add(time.Hour)
	restarted.change("four", false, later)
	restarted.change("five", false, later)
	restarted.change("six", false, later)
	waitForHistory(t, restarted)
	if stats := restarted.snapshot(later); stats.Peak != 3 || stats.HistoricalPeak != 3 || stats.HistoricalPeakAt != later.Unix() {
		t.Fatalf("higher peak was not recorded: %+v", stats)
	}
	reloaded := newConcurrencyMeterAt(path)
	stats = reloaded.snapshot(later.Add(time.Second))
	if stats.Peak != 0 || stats.HistoricalPeak != 3 || stats.HistoricalPeakAt != later.Unix() || len(stats.Points) == 0 || stats.Points[0].Peak != 0 {
		t.Fatalf("window reset must not clear historical peak: %+v", stats)
	}
}

func waitForHistory(t *testing.T, meter *concurrencyMeter) {
	t.Helper()
	meter.Lock()
	saving := meter.historySaving
	meter.Unlock()
	if saving != nil {
		select {
		case <-saving:
		case <-time.After(5 * time.Second):
			t.Fatal("historical peak save did not finish")
		}
	}
}

func TestConcurrencyLegacySchemaKeepsMonitoringDisabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	defer requestMeter.Store(nil)
	if _, err := handleMethod("plugin.register", []byte(`{"schema_version":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := handleMethod("request.intercept_before", []byte(`{"RequestID":"legacy"}`)); err != nil {
		t.Fatal(err)
	}
	raw, err := handleMethod("management.handle", []byte(`{"Method":"GET","Path":"/v0/management/plugins/codex-ny-timezone/concurrency"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Result concurrencyResourceResponse }
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.StatusCode != 503 {
		t.Fatalf("legacy stats status = %d, want 503", result.Result.StatusCode)
	}
}

func TestConcurrencyKeepsUnreadableHistoryAndRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	meter := newConcurrencyMeterAt(path)
	start := time.Unix(1800000000, 0)
	meter.change("one", false, start)
	waitForHistory(t, meter)
	if stats := meter.snapshot(start); stats.HistoricalPersisted || stats.HistoricalStatus != "read_error" || stats.Peak != 1 {
		t.Fatalf("invalid history should not stop live counts or appear saved: %+v", stats)
	}
	waitForHistory(t, meter)
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "{" {
		t.Fatalf("invalid old data was replaced: %q, %v", raw, err)
	}
	if err := os.Rename(path, path+".bak"); err != nil {
		t.Fatal(err)
	}
	meter.snapshot(start)
	waitForHistory(t, meter)
	if stats := meter.snapshot(start); !stats.HistoricalPersisted || stats.HistoricalPeak != 1 {
		t.Fatalf("pending peak was not saved after storage recovery: %+v", stats)
	}
}

func TestConcurrencyOlderMeterCannotOverwriteHigherPeak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	later, earlier := newConcurrencyMeterAt(path), newConcurrencyMeterAt(path)
	start := time.Unix(1800000000, 0)
	earlier.change("first", false, start)
	earlier.change("second", false, start)
	waitForHistory(t, earlier)
	later.change("third", false, start.Add(time.Second))
	waitForHistory(t, later)
	saved, err := readHistoricalPeak(path)
	if err != nil || saved.Peak != 2 || saved.PeakAt != start.Unix() {
		t.Fatalf("lower peak overwrote history: %+v, %v", saved, err)
	}
}
