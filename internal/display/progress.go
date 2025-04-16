package display

type ProgressReporter interface {
    Update(add int, addMax int, droppedServers int)
    Finish()
}
