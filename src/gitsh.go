package gitreviewers

import (
	"os/exec"
	rx "regexp"
	"strconv"
	"strings"
)

var countExtractor *rx.Regexp

func init() {
	countExtractor = rx.MustCompile("(\\d+)\\s*(.*)$")
}

func runCommand(command string) (string, error) {
	// TODO Output command in verbose mode
	tokens := strings.Split(command, " ")
	out, err := exec.Command(tokens[0], tokens[1:]...).CombinedOutput()

	if err != nil {
		// TODO Output error in verbose mode
		return "", err
	}

	return string(out), nil
}

func commitTimeStamp(branch string) (string, error) {
	out, err := runCommand("git show --format=\"%ct\" " + branch)
	if err != nil {
		return "", nil
	}

	line := strings.Split(out, "\n")[0]
	return strings.Trim(line, "\""), nil
}

func changedFiles() ([]string, error) {
	var lines []string
	out, err := runCommand("git diff master HEAD --name-only")

	if err != nil {
		return lines, err
	}

	for _, line := range strings.Split(out, "\n") {
		l := strings.Trim(line, " ")
		if len(l) > 0 {
			lines = append(lines, l)
		}
	}

	return lines, err
}

func committerCounts(path string) ([]CommitterStat, error) {
	var stats []CommitterStat

	sinceCommit, err := exec.Command(
		"bash", "-c", "git log --since 2015-01-01 --reverse |"+
			"head -n 1 | awk '{print $2}'").Output()

	if err != nil {
		return stats, err
	}

	cmd := strings.Join(
		[]string{
			"git shortlog -sne --no-merges",
			strings.TrimSpace(string(sinceCommit)) + "..HEAD",
			path,
		}, " ")

	out, err := runCommand(cmd)
	if err != nil {
		return stats, err
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

		stats = append(stats, CommitterStat{rvwr, cti})
	}

	return stats, nil
}
