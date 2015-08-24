package gitreviewers

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestChangedFiles(t *testing.T) {
	// Set up a fake commit in a fake branch
	tfName := "fake.co"
	var safeToReset = false

	// Get current branch
	out, e := exec.Command("git", "status", "-sb").Output()
	if e != nil {
		t.Error("Issue getting current branch")
		t.FailNow()
	}
	// Find the newline
	nlPos := 0
	for i, b := range out {
		nlPos = i
		if b == '\n' {
			break
		}
	}
	// git status -sb format:
	// ## branch_name\nsome other stuff
	branch := string(out[3:nlPos])

	if err := exec.Command("git", "checkout", "-b", "fake-branch").Run(); err != nil {
		t.Error("Issue creating new branch. Please clean up!")
		t.FailNow()
	}

	var f *os.File
	var err error

	defer func(safeToReset *bool, f *os.File, branch string) {
		if f != nil {
			_ = os.Remove(f.Name())
		}

		if *safeToReset {
			if err := exec.Command("git", "checkout", branch).Run(); err != nil {
				t.Error("Issue switching back to master. Please clean up!")
				t.FailNow()
			}

			if err := exec.Command("git", "branch", "-D", "fake-branch").Run(); err != nil {
				t.Error("Issue destroying test branch. Please clean up!")
				t.FailNow()
			}
		}
	}(&safeToReset, f, branch)

	f, err = os.Create(tfName)
	if err != nil {
		t.Error("Issue setting up fake commit file. Please clean up!")
		t.FailNow()
	}

	if err := exec.Command("git", "add", tfName).Run(); err != nil {
		t.Error("Issue staging the commit. Please clean up!")
		t.FailNow()
	}

	if err := exec.Command("git", "commit", "-m", "\"Fake commit\"").Run(); err != nil {
		t.Error("Issue committing. Please clean up!")
		t.FailNow()
	}
	safeToReset = true

	// Test for changes
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

	ch := make(chan Stat)
	done := make(chan statResp)

	var err error
	var counts []Stat

	go func(path string) {
		committerCounts(path, ch, done)
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
		committerCounts(path, ch, done)
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
