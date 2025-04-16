package display

import (
	// standard
	"fmt"
	"os"
	"time"
	// external
	"github.com/schollz/progressbar/v3"
	// local
)

type TTYProgressReporter struct {
	bar *progressbar.ProgressBar
	barDescription string
	totalDroppedServers int
}


// constructor
func NewTTYProgressReporter(
	color string, total int,
) *TTYProgressReporter {

	// barDescription := "    " + color + "%d/%d reqs (%d failed servers)"
	barDescription := "    " + color + "%d/%d req (-%d srv)"
	bar := progressbar.NewOptions(
		total, // initial 'max' for progress bar
		progressbar.OptionSetWriter(os.Stderr), // write on stderr
		progressbar.OptionThrottle(time.Second / 3),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetWidth(30),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("req"),
		progressbar.OptionSetDescription(fmt.Sprintf(barDescription, 0, 0, 0)),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	return &TTYProgressReporter{
		bar: bar,
		barDescription: barDescription,
		totalDroppedServers: 0,
	}
}


func (r *TTYProgressReporter) Update(
	add int, addMax int, droppedServers int,
) {
	st := r.bar.State()
	newCurrentNum := int(st.CurrentNum) + add
	newMax := int(st.Max) + addMax
	if addMax != 0 {
		r.bar.ChangeMax(newMax)
	}
	if droppedServers != 0 {
		r.totalDroppedServers += droppedServers
	}
	if add != 0 {
		// describe BEFORE finish, otherwise total is not redrawn at end:
		if newCurrentNum >= newMax {
			r.bar.Describe(fmt.Sprintf(
				r.barDescription, newCurrentNum, newMax, r.totalDroppedServers))
			// fmt.Printf("\n\n%d + %d = %d\n\n", st.CurrentNum, add, st.Max)
		}
		r.bar.Add(add)
	}
	if add != 0 || addMax != 0 || droppedServers != 0 {
		r.bar.Describe(fmt.Sprintf(
			r.barDescription, st.CurrentNum, st.Max, r.totalDroppedServers))
	}
}


func (r *TTYProgressReporter) Finish() {
	fmt.Fprintf(os.Stderr, "\033[0m\n")
}
