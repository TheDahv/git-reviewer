package gitreviewers

// runGuard supports programming with the "sticky errors" pattern, allowing
// a chain of operations with potential failures to run through, skipping
// downstream operations if an upstream operation returns an error.
// Clients can inspect the error and specify an optional message of
// what step created the error with `msg`.
type runGuard struct {
	err error
	msg string
}

// maybeRun runs the operation inside `fn`, but only if an existing error
// has not been stored. `fn` should be a closure that has outside variables,
// including this runGuard instance, in scope. It should set `err`, and an
// optional `msg` describing which step in the pipline failed if its contents
// create an error.
func (rg *runGuard) maybeRun(fn func()) {
	if rg.err != nil {
		return
	}
	fn()
}
