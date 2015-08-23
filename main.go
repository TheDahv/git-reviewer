package main

import (
	"fmt"

	"github.com/thedahv/git-reviewer/src"
)

func main() {
	// Find changed files in this branch.
	files, err := gitreviewers.FindFiles()

	if err != nil {
		fmt.Printf("There was an error finding files: %v\n", err)
		return
	}

	fmt.Println("Reviewers across the following changed files:")
	for _, file := range files {
		fmt.Printf("  %s\n", file)
	}

	// Find the best reviewers for these files.
	reviewers, err := gitreviewers.FindReviewers(files)
	if err != nil {
		fmt.Printf("There was an error finding reviewers: %v\n", err)
		return
	}

	fmt.Printf("\nReviewers:\n")
	for _, reviewer := range reviewers {
		fmt.Println(reviewer)
	}
}
