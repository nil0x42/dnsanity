package report

import (
	"time"
)


// A batch of N requests observed at a given moment.
type RequestsBatch struct {
	time	time.Time
	count	int
}

// A log is just a growable slice of batches.
type RequestsLog []RequestsBatch

// Add one batch of Requests made at a specific time
func (r *RequestsLog) LogRequests(t time.Time, n int) {
	*r = append(*r, RequestsBatch{time: t, count: n})
}

// Return the number of requests in the last second
// and drop all older batches.
func (r *RequestsLog) CountLastSecRequests() int {
	now := time.Now()
	cutoff := now.Add(-time.Second)

	s := *r
	write := 0
	sum := 0

	for _, b := range s {
		if b.time.After(cutoff) {
			s[write] = b
			write++
			sum += b.count
		}
	}
	*r = s[:write] // drop batches older than 1sec
	return sum
}
