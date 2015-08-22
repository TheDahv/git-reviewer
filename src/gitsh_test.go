package gitreviewers

import "testing"

func TestChangedFiles(t *testing.T) {
	lines, err := changedFiles()

	if err != nil {
		t.Errorf("Got error %v, expected none\n", err)
	}

	if len(lines) == 0 {
		t.Error("Got 0 lines, expected more")
	}
}
