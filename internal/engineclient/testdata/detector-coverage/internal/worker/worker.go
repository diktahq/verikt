package worker

// Spawn is deliberately not named Start, Run, Serve or ListenAndServe: those are
// treated as server lifecycle methods and exempt from the detector.
func Spawn() {
	go func() {
		process()
	}()
}

func process() {}
