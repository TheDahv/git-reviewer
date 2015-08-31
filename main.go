package main

import (
	"errors"
	"flag"
	"fmt"
	"regexp"

	gr "github.com/thedahv/git-reviewer/src"
)

// dateRx helps us ensure date arguments confirm to YYYY-MM-DD format
var dateRx *regexp.Regexp

func init() {
	dateRx = regexp.MustCompile("\\d{4}-\\d{2}-\\d{2}")
}

func main() {
	showFiles := flag.Bool("show-files", false, "Show changed files for reviewing")
	verbose := flag.Bool("verbose", false, "Show progress and errors information")
	force := flag.Bool("force", false, "Continue processing despite checks or errors")
	since := flag.String("since", "", "Consider commits after date when finding"+
		" reviewers. Defaults to 6 months ago (format 'YYYY-MM-DD')")

	flag.Parse()

	err := checkDateArg(*since)
	if len(*since) > 0 && err != nil {
		fmt.Println("Problem with input format for 'since' argument. Run 'git reviewer -h'")
		return
	}

	r := gr.Reviewer{*showFiles, *verbose, *since}

	// Determine if branch is reviewable
	if behind, err := r.BranchBehind(); behind || err != nil {
		if err != nil {
			fmt.Printf("There was an error determining branch state: %v\n", err)
			return
		}

		fmt.Print("Current branch is behind master. Merge up!")
		if *force {
			return
		}
	}

	// Find changed files in this branch.
	files, err := r.FindFiles()

	if err != nil {
		fmt.Printf("There was an error finding files: %v\n", err)
		return
	}

	if len(files) == 0 {
		fmt.Println("No changes on this branch!")
		return
	}

	if *showFiles {
		fmt.Println("Reviewers across the following changed files:")
		for _, file := range files {
			fmt.Printf("  %s\n", file)
		}
		fmt.Println()
	}

	// Find the best reviewers for these files.
	reviewers, err := r.FindReviewers(files)
	if err != nil {
		fmt.Printf("There was an error finding reviewers: %v\n", err)
		return
	}

	fmt.Printf("Reviewers:\n")
	for _, reviewer := range reviewers {
		fmt.Println(reviewer)
	}
}

// checkDateArg takes a date argument as a YYYY-MM-DD formatted string and
// ensures it is appropriate for usage in the program. Returns an error
// if any checks fail, or nil if things look fine.
func checkDateArg(input string) error {
	if len(input) == 0 {
		return errors.New("no input")
	}

	r := dateRx.Find([]byte(input))

	if r == nil {
		return errors.New("input doesn't match expected format (YYYY-MM-DD)")
	}

	// TODO Make sure date arg isn't greater than today

	return nil
}
