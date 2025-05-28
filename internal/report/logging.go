package report

import (
	"os"
	"io"
	"time"
)


/* ------------------------------------------------------------------ */
/* IOFiles ---------------------------------------------------------- */
/* ------------------------------------------------------------------ */

type IOFiles struct {
	TTYFile		*os.File
	OutputFile	io.Writer
	VerboseFile	io.Writer
	DebugFile	io.Writer
}


/* ------------------------------------------------------------------ */
/* MetricGauge ------------------------------------------------------ */
/* ------------------------------------------------------------------ */

// MetricGauge tracks an integer metric (current, peak, average).
// Not concurrency-safe.
type MetricGauge struct {
	Max       int   // optional: theoratical max
	Current   int   // last recorded value
	Peak      int   // highest ever recorded value
	totalSum  int64 // sum of all values logged
	nSamples  int64 // number of calls to Log()
}

// Log records one sample and updates Peak / average.
func (g *MetricGauge) Log(v int) {
	g.Current = v
	if v > g.Peak {
		g.Peak = v
	}
	g.totalSum += int64(v)
	g.nSamples++
}

// Avg returns the rounded arithmetic mean of all samples.
func (g *MetricGauge) Avg() int {
	if g.nSamples == 0 {
		return 0
	}
	return int(float64(g.totalSum)/float64(g.nSamples) + 0.5)
}

/* ------------------------------------------------------------------ */
/* RequestsLogger --------------------------------------------------- */
/* ------------------------------------------------------------------ */

// RequestsBatch represents the number of requests observed at one instant.
type RequestsBatch struct {
	timestamp time.Time
	count     int
}

// RequestsLogger tracks total idle / busy requests and
// keeps a sliding-window log (1 s) to compute RPS.
type RequestsLogger struct {
	StartTime  time.Time       // start time
	Idle       int             // cumulative idle requests
	Busy       int             // cumulative busy requests
	OneSecPeak int             // highest observers 1s RPS
	batches    []RequestsBatch // sliding window of ≤ 1 s
}

// Log records a new batch of requests and prunes outdated entries.
//
// idleDelta / busyDelta: number of idle / busy requests since the last call.
// ts: timestamp of the observation (usually time.Now()).
func (r *RequestsLogger) Log(ts time.Time, idleDelta, busyDelta int) {
	// Update cumulative counters.
	r.Idle += idleDelta
	r.Busy += busyDelta
	// Store the batch for 1-second sliding window (RPS).
	total := idleDelta + busyDelta
	if total > 0 {
		r.batches = append(
			r.batches, RequestsBatch{timestamp: ts, count: total})
	}
	// Prune batches older than 1 s to avoid unbounded growth even
	// when LastSecCount() is never called.
	cutoff := ts.Add(-time.Second)
	keep := 0
	for _, b := range r.batches {
		if b.timestamp.After(cutoff) {
			r.batches[keep] = b
			keep++
		}
	}
	r.batches = r.batches[:keep]
}

// Total returns the overall number of requests logged since program start.
func (r *RequestsLogger) Total() int {
	return r.Idle + r.Busy
}

// LastSecCount returns the number of requests in the last second and
// purges everything older than that window.
//
// This is O(k) where k = len(batches) ≤ number of events within 1 s.
func (r *RequestsLogger) LastSecCount() int {
	now := time.Now()
	cutoff := now.Add(-time.Second)

	sum := 0
	keep := 0
	for _, b := range r.batches {
		if b.timestamp.After(cutoff) {
			r.batches[keep] = b
			keep++
			sum += b.count
		}
	}
	r.batches = r.batches[:keep]
	if sum > r.OneSecPeak {
		r.OneSecPeak = sum
	}
	return sum
}

// OneSecAvg returns the average requests-per-second (RPS) since StartTime.
func (r *RequestsLogger) OneSecAvg() int {
	elapsedUs := time.Since(r.StartTime).Microseconds() + 500_000 // +500ms
	if elapsedUs <= 0 {
		return 0
	}
	return int(0.5 + (float64(r.Total() * 1_000_000) / float64(elapsedUs)))
}
