package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const concurrencyStatsRoute = "/concurrency"
const sampleSeconds = 5
const historySize = 120
const recentCompletions = 4096
const historyDirectory = "codex-ny-timezone"
const historyFile = "concurrency-history.json"

// A new registration can arrive while the previous meter is still saving.
var historyFileMu sync.Mutex

type concurrencyPoint struct {
	Tick int64 `json:"tick"`
	Peak int   `json:"peak"`
}

type concurrencyStats struct {
	Active              int                `json:"active"`
	Peak                int                `json:"peak"`
	PeakAt              int64              `json:"peak_at"`
	HistoricalPeak      int                `json:"historical_peak"`
	HistoricalPeakAt    int64              `json:"historical_peak_at"`
	HistoricalPersisted bool               `json:"historical_persisted"`
	HistoricalStatus    string             `json:"historical_status"`
	Tick                int64              `json:"tick"`
	Points              []concurrencyPoint `json:"points"`
}

type persistedConcurrencyStats struct {
	Peak   int   `json:"peak"`
	PeakAt int64 `json:"peak_at"`
}

type concurrencyMeter struct {
	sync.Mutex
	active           map[string]struct{}
	peak             int
	peakAt           int64
	first            int64
	last             int64
	points           [historySize]concurrencyPoint
	finished         map[string]struct{}
	finishedOrder    [recentCompletions]string
	finishedNext     int
	historyPath      string
	historicalPeak   int
	historicalAt     int64
	savedPeak        int
	historicalStatus string
	historySaving    chan struct{}
}

func newConcurrencyMeter() *concurrencyMeter {
	path := ""
	if dir, err := os.UserConfigDir(); err == nil {
		path = filepath.Join(dir, historyDirectory, historyFile)
	}
	return newConcurrencyMeterAt(path)
}

func newConcurrencyMeterAt(path string) *concurrencyMeter {
	meter := &concurrencyMeter{
		active: make(map[string]struct{}), finished: make(map[string]struct{}), historyPath: path,
	}
	if path == "" {
		meter.historicalStatus = "unavailable"
		return meter
	}
	historyFileMu.Lock()
	defer historyFileMu.Unlock()
	saved, err := readHistoricalPeak(path)
	meter.historicalStatus = "ready"
	if err != nil {
		meter.historicalStatus = "read_error"
	}
	meter.historicalPeak, meter.historicalAt, meter.savedPeak = saved.Peak, saved.PeakAt, saved.Peak
	return meter
}

func readHistoricalPeak(path string) (persistedConcurrencyStats, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return persistedConcurrencyStats{}, nil
	}
	if err != nil {
		return persistedConcurrencyStats{}, err
	}
	var saved persistedConcurrencyStats
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Peak <= 0 || saved.PeakAt <= 0 {
		return persistedConcurrencyStats{}, errors.New("invalid historical peak record")
	}
	return saved, nil
}

// Call under the meter lock so only one writer needs to catch up with new peaks.
func (m *concurrencyMeter) queueHistoricalPeak() {
	if m.historyPath == "" || m.historySaving != nil || m.historicalPeak <= m.savedPeak {
		return
	}
	m.historySaving = make(chan struct{})
	go m.saveHistoricalPeak()
}

func (m *concurrencyMeter) flushHistoricalPeak() {
	m.Lock()
	m.queueHistoricalPeak()
	saving := m.historySaving
	m.Unlock()
	if saving != nil {
		<-saving
	}
}

func (m *concurrencyMeter) saveHistoricalPeak() {
	m.Lock()
	for {
		next := persistedConcurrencyStats{Peak: m.historicalPeak, PeakAt: m.historicalAt}
		m.Unlock()
		saved, status := storeHistoricalPeak(m.historyPath, next)
		m.Lock()
		m.historicalStatus = status
		if status == "ready" {
			m.savedPeak = saved.Peak
			if saved.Peak >= m.historicalPeak {
				m.historicalPeak, m.historicalAt = saved.Peak, saved.PeakAt
			}
		}
		if status != "ready" || m.savedPeak == m.historicalPeak {
			break
		}
	}
	close(m.historySaving)
	m.historySaving = nil
	m.Unlock()
}

func storeHistoricalPeak(path string, next persistedConcurrencyStats) (persistedConcurrencyStats, string) {
	historyFileMu.Lock()
	defer historyFileMu.Unlock()
	saved, err := readHistoricalPeak(path)
	if err != nil {
		return saved, "read_error"
	}
	if saved.Peak >= next.Peak {
		return saved, "ready"
	}
	if err := replaceHistoricalPeak(path, next); err != nil {
		return saved, "write_error"
	}
	return next, "ready"
}

func replaceHistoricalPeak(path string, peak persistedConcurrencyStats) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".concurrency-history-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(peak)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (m *concurrencyMeter) advance(tick int64) {
	if m.first == 0 {
		m.first = tick
		m.last = tick - 1
	}
	if tick <= m.last {
		return
	}
	for next := max(m.last+1, tick-historySize+1); next <= tick; next++ {
		m.points[next%historySize] = concurrencyPoint{Tick: next, Peak: len(m.active)}
	}
	m.last = tick
}

func (m *concurrencyMeter) change(id string, complete bool, now time.Time) {
	if id == "" {
		return
	}
	m.Lock()
	defer m.Unlock()
	if _, finished := m.finished[id]; finished {
		return
	}
	m.advance(now.Unix() / sampleSeconds)
	if complete {
		delete(m.active, id)
		// CPA cancellation can deliver completion before the detached native call returns.
		delete(m.finished, m.finishedOrder[m.finishedNext])
		m.finishedOrder[m.finishedNext] = id
		m.finishedNext = (m.finishedNext + 1) % recentCompletions
		m.finished[id] = struct{}{}
		return
	}
	m.active[id] = struct{}{}
	active := len(m.active)
	point := &m.points[m.last%historySize]
	point.Peak = max(point.Peak, active)
	if active > m.peak {
		m.peak, m.peakAt = active, now.Unix()
	}
	if active > m.historicalPeak {
		m.historicalPeak, m.historicalAt = active, now.Unix()
		m.queueHistoricalPeak()
	}
}

func (m *concurrencyMeter) snapshot(now time.Time) concurrencyStats {
	m.Lock()
	defer m.Unlock()
	m.advance(now.Unix() / sampleSeconds)
	m.queueHistoricalPeak()
	status := m.historicalStatus
	if status == "ready" && m.savedPeak < m.historicalPeak {
		status = "pending"
	}
	stats := concurrencyStats{
		Active: len(m.active), Peak: m.peak, PeakAt: m.peakAt,
		HistoricalPeak: m.historicalPeak, HistoricalPeakAt: m.historicalAt,
		HistoricalPersisted: status == "ready" && m.historicalPeak > 0, HistoricalStatus: status, Tick: m.last,
	}
	for tick := max(m.first, m.last-historySize+1); tick <= m.last; tick++ {
		stats.Points = append(stats.Points, m.points[tick%historySize])
	}
	return stats
}

// The host authenticates exact management routes before invoking the plugin.
func serveConcurrencyStats(meter *concurrencyMeter) ([]byte, error) {
	status := http.StatusServiceUnavailable
	payload := any(map[string]string{"error": "当前 CPA 不支持请求生命周期统计。"})
	if meter != nil {
		status, payload = http.StatusOK, meter.snapshot(time.Now())
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return okEnvelope(concurrencyResourceResponse{StatusCode: status, Body: body, Headers: http.Header{
		"Content-Type": {"application/json; charset=utf-8"}, "Cache-Control": {"no-store"},
	}})
}
