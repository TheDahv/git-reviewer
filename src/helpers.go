package gitreviewers

import (
	"bufio"
	"bytes"
	"fmt"
	"os"

	gg "github.com/libgit2/git2go"
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

type mailmap map[string]string

func readMailmap() (mailmap, error) {
	var (
		rg  runGuard
		cwd string
		mm  = make(mailmap)
	)

	rg.maybeRun(func() {
		var err error

		cwd, err = os.Getwd()
		if err != nil {
			rg.err = err
			rg.msg = "Error determining current working directory"
		}
	})

	// Check for profile mailmap
	rg.maybeRun(func() {
		cp, err := gg.ConfigFindGlobal()
		if err != nil {
			fmt.Printf("Error finding global config: %v\n", err)
			return
		}

		conf, err := gg.OpenOndisk(&gg.Config{}, cp)
		if err != nil {
			return
		}
		defer conf.Free()

		path, err := conf.LookupString("mailmap.file")
		if err != nil {
			fmt.Printf("Error lookup up mailmap file %v\n", err)
			return
		}

		f, err := os.Open(path)
		defer f.Close()
		if err != nil {
			return
		}

		readMailmapFromFile(mm, f)
		useMailmap = true
	})

	// Parse project mailmap last so it overrides
	rg.maybeRun(func() {
		f, err := os.Open(cwd + ".mailmap")
		defer f.Close()
		if err != nil {
			return
		}

		readMailmapFromFile(mm, f)
	})

	// Check for any errors that might have happened
	if rg.err != nil {
		return nil, rg.err
	}

	return mm, nil
}

func readMailmapFromFile(mm map[string]string, f *os.File) {
	// See git C implementation of parse_name_and_email for reference
	// https://github.com/git/git/blob/master/mailmap.c
	var (
		line    []byte
		err     error
		lastPos int
	)
	var name1, email1, name2, email2 string

	rdr := bufio.NewReader(f)

	for {
		line, err = rdr.ReadBytes('\n')
		if err != nil {
			// TODO Handle non-EOF errors
			break
		}

		if line[0] != '#' {
			name1, email1, lastPos = parseMailmapLine(line, 0)

			if lastPos > 0 {
				name2, email2, _ = parseMailmapLine(line, lastPos)

				mm[name2] = name1
				mm[email2] = email1
			}
		}
		// TODO Implement repo-abbrev parsing. I have no idea what that is
	}
}

func parseMailmapLine(line []byte, offset int) (name string, email string, right int) {
	var left int

	left = bytes.IndexRune(line[offset:], '<')
	if left < 0 {
		return
	}

	right = bytes.IndexRune(line[offset+left:], '>')
	if right < 0 {
		return
	}
	// Account for the fact we got the index of a sub-slice
	right = left + right

	name = string(bytes.TrimSpace(line[:offset+left]))
	email = string(bytes.TrimSpace(line[offset+left+1 : offset+right]))

	return
}
