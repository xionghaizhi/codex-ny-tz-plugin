package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const concurrencyStatsRoute = "/concurrency"
const sampleSeconds = 5
const historySize = 120
const recentCompletions = 4096

type concurrencyPoint struct {
	Tick int64 `json:"tick"`
	Peak int   `json:"peak"`
}

type concurrencyStats struct {
	Active int                `json:"active"`
	Peak   int                `json:"peak"`
	PeakAt int64              `json:"peak_at"`
	Tick   int64              `json:"tick"`
	Points []concurrencyPoint `json:"points"`
}

type concurrencyMeter struct {
	sync.Mutex
	active        map[string]struct{}
	peak          int
	peakAt        int64
	first         int64
	last          int64
	points        [historySize]concurrencyPoint
	finished      map[string]struct{}
	finishedOrder [recentCompletions]string
	finishedNext  int
}

func newConcurrencyMeter() *concurrencyMeter {
	return &concurrencyMeter{active: make(map[string]struct{}), finished: make(map[string]struct{})}
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
}

func (m *concurrencyMeter) snapshot(now time.Time) concurrencyStats {
	m.Lock()
	defer m.Unlock()
	m.advance(now.Unix() / sampleSeconds)
	stats := concurrencyStats{Active: len(m.active), Peak: m.peak, PeakAt: m.peakAt, Tick: m.last}
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
