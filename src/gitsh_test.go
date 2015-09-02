package gitreviewers

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestCommitterCounts(t *testing.T) {
	path := os.Getenv("GOPATH") +
		"/src/github.com/thedahv/git-reviewer/src"

	ch := make(chan Stat)
	done := make(chan statResp)

	var err error
	var counts []Stat

	go func(path string) {
		committerCounts(path, "", ch, done)
	}(path)

	for i := 0; i < 1; {
		select {
		case stat := <-ch:
			counts = append(counts, stat)
		case signal := <-done:
			err = signal.err
			i++
		}
	}

	close(ch)
	close(done)

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

func TestCommitterCountsOnBadPath(t *testing.T) {
	path := "doesn't-exist"

	ch := make(chan Stat)
	done := make(chan statResp)

	var err error
	var counts []Stat

	go func() {
		committerCounts(path, "", ch, done)
	}()

	for i := 0; i < 1; {
		select {
		case stat := <-ch:
			counts = append(counts, stat)
		case signal := <-done:
			err = signal.err
			i++
		}
	}

	if err == nil {
		t.Error("Got no error back, expected one")
	}

	if len(counts) != 0 {
		t.Errorf("Expected no stats back, got %d\n", len(counts))
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
