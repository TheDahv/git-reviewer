package gitreviewers

import (
	"fmt"
	"os/exec"
	rx "regexp"
	"strconv"
	"strings"
)

var countExtractor *rx.Regexp

func init() {
	countExtractor = rx.MustCompile("(\\d)+\\s*(.*)$")
}

func runCommand(command string) (string, error) {
	tokens := strings.Split(command, " ")
	out, err := exec.Command(tokens[0], tokens[1:]...).CombinedOutput()

	if err != nil {
		fmt.Println("Git error!")
		fmt.Println(string(out))
		return "", err
	}

	return string(out), nil
}

func commitTimeStamp(branch string) (string, error) {
	return runCommand("git show --format=\"%ct\" " + branch)
}

func changedFiles() ([]string, error) {
	var lines []string
	out, err := runCommand("git diff master HEAD --name-only")

	if err != nil {
		return lines, err
	}

	lines = strings.Split(out, "\n")

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

	lines := strings.Split(out, "\n")
	for _, line := range lines {
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
