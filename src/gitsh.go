package gitreviewers

import (
	"os/exec"
	rx "regexp"
	"strings"
)

var countExtractor *rx.Regexp

func init() {
	// Pattern to extract commit count and name/email from git shortlog.
	countExtractor = rx.MustCompile("(\\d+)\\s*(.*)$")
}

// run executes cmd via a shell process and returns
// its output as a string. If the shell returns an error, return
// that instead.
func run(cmd string) (string, error) {
	// TODO Output command in verbose mode
	tokens := strings.Split(cmd, " ")
	out, err := exec.Command(tokens[0], tokens[1:]...).CombinedOutput()

	if err != nil {
		// TODO Output error in verbose mode
		return "", err
	}

	return string(out), nil
}
