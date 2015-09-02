package gitreviewers

import (
	"os/exec"
	rx "regexp"
	"strconv"
	"strings"
	"time"
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

// commitTimeStamp returns the timestamp of the current commit for
// the object (branch, commit, etc.).
func commitTimeStamp(obj string) (string, error) {
	out, err := run("git show --format=\"%ct\" " + obj)
	if err != nil {
		return "", nil
	}

	line := strings.Split(out, "\n")[0]
	return strings.Trim(line, "\""), nil
}

// committerCounts finds recent committers and commit counts for
// the file at `path`. It uses 2 channels to communicate the state of
// processing. If `since` is a proper 'YYYY-MM-DD' formatted date, the
// command will only consider commits for `path` created after the date.
// Otherwise, it defaults to 6 months before the current day.
//
// `stat` emits committer statistics as they are found for each file.
// `done` emits once with a possible error to signal completion.
func committerCounts(path string, since string, stat chan Stat, done chan statResp) {
	var signal = statResp{path: path}

	if len(since) == 0 {
		// Calculate 6 months ago from today's date and set the 'since' argument
		since = time.Now().AddDate(0, -6, 0).Format("2006-01-02")
	}

	c, err := exec.Command(
		"bash", "-c", "git log --since "+since+" --reverse |"+
			"head -n 1 | awk '{print $2}'").Output()

	if err != nil {
		signal.err = err
		done <- signal
	}

	cmd := strings.Join(
		[]string{
			"git shortlog -sne --no-merges",
			strings.TrimSpace(string(c)) + "..HEAD",
			path,
		}, " ")

	out, err := run(cmd)
	if err != nil {
		signal.err = err
		done <- signal
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.Trim(line, " ")
		matches := countExtractor.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}

		ct := matches[1]
		rvwr := matches[2]

		cti, err := strconv.Atoi(ct)
		if err != nil {
			continue
		}

		stat <- Stat{rvwr, cti}
	}

	done <- signal
}
