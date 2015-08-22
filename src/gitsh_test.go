package gitreviewers

import (
	"os"
	"testing"
)

func TestChangedFiles(t *testing.T) {
	lines, err := changedFiles()

	if err != nil {
		t.Errorf("Got error %v, expected none\n", err)
	}

	if len(lines) == 0 {
		t.Error("Got 0 lines, expected more")
	}
}

func TestCommitterCounts(t *testing.T) {
	path := os.Getenv("GOPATH") +
		"/src/github.com/thedahv/git-reviewer/src"

	counts, err := committerCounts(path)

	if err != nil {
		t.Errorf("Got error %v, expected none\n", err)
		t.FailNow()
	}

	if len(counts) == 0 {
		t.Errorf("Got 0 counts, expected more")
		t.FailNow()
	}

	if counts[0].Reviewer == "" || counts[0].Count == 0 {
		t.Errorf("Got empty stats where we didn't expect to")
	}
}
