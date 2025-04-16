package display

import (
	"fmt"
	"os"
)

type NoTTYProgressReporter struct {
	doneReqs            int
	totalReqs           int
	totalDroppedServers int
}


// constructor
func NewNoTTYProgressReporter(
	total int,
) *NoTTYProgressReporter {
	fmt.Fprintf(
		os.Stderr,
		"    Starting (%d reqs scheduled so far) ...\n",
		total,
	)
	return &NoTTYProgressReporter{
		doneReqs: 0,
		totalReqs: total,
	}
}


func (r *NoTTYProgressReporter) Update(
	add int, addMax int, droppedServers int,
) {
	r.doneReqs += add
	r.totalReqs += addMax
	r.totalDroppedServers += droppedServers
}


func (r *NoTTYProgressReporter) Finish() {
	fmt.Fprintf(
		os.Stderr,
		"    Finished (%d/%d reqs done).\n",
		r.doneReqs, r.totalReqs)
	}
