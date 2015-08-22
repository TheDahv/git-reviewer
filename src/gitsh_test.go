package gitreviewers

import (
	"os"
	"strconv"
	"testing"
	"time"
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

func TestCommitTimestamp(t *testing.T) {
	ts, err := commitTimeStamp("master")

	if err != nil {
		t.Errorf("Got error %v, expected none\n", err)
		t.FailNow()
	}

	tsi, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		t.Errorf("Unable to turn timestamp into integer: %v\n", err)
	}
	// As long as we parse into some kind of date without issue, we're ok
	time.Unix(tsi, 0)
}
