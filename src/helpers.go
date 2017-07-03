package gitreviewers

import (
	"bufio"
	"bytes"
	"io"
	"os"
)

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

// maybeRunMany runs the series of operations inside `fn` one after the other.
// If any function in the sequence saves an error on the run guard, the sequence
// stops and skips all downstream operations. The final error and an optional
// message explaining which step in the sequence failed are available to client
// code.
func (rg *runGuard) maybeRunMany(fns ...func()) {
	for _, fn := range fns {
		if rg.err != nil {
			break
		}
		fn()
	}
}

type mailmap map[string]string

func readMailmap(paths []string) (mailmap, error) {
	mm := make(mailmap)

	for _, p := range paths {
		if f, err := os.Open(p); err == nil {
			readMailmapFromSource(mm, f)
			f.Close()
		}
	}

	return mm, nil
}

func readMailmapFromSource(mm mailmap, src io.Reader) error {
	// See git C implementation of parse_name_and_email for reference
	// https://github.com/git/git/blob/master/mailmap.c
	scanner := bufio.NewScanner(src)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Skip comments and blank lines
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		name1, email1, read := parseMailmapLine(line, 0)

		// Simple unaliased mapping: e.g. "Name <email>"
		if len(name1) > 0 {
			mm[name1] = name1
		}
		if len(email1) > 0 {
			mm[email1] = email1
		}

		if read > 0 {
			name2, email2, _ := parseMailmapLine(line, read)

			if len(name1) > 0 {
				if len(name2) > 0 {
					mm[name2] = name1
				} else {
					mm[name1] = name1
				}
			}

			if len(email1) > 0 {
				if len(email2) > 0 {
					mm[email2] = email1
				} else {
					mm[email1] = email1
				}
			}
		}

		// TODO Implement repo-abbrev parsing. I have no idea what that is
	}

	if err := scanner.Err(); err == nil || err == io.EOF {
		return nil
	} else {
		return err
	}
}

func parseMailmapLine(line []byte, offset int) (name string, email string, right int) {
	var left int

	left = bytes.IndexRune(line[offset:], '<')
	if left < 0 {
		return
	}

	right = bytes.IndexRune(line[offset:], '>')
	if right < 0 {
		return
	}

	name = string(bytes.TrimSpace(line[offset : offset+left]))
	email = string(bytes.TrimSpace(line[offset+left+1 : offset+right]))

	// Turn a 0-based index into a length and account for offset
	right = right + offset + 1

	return
}
