package main

import (
	"flag"
	"fmt"

	gr "github.com/thedahv/git-reviewer/src"
)

func main() {
	showFiles := flag.Bool("show-files", false, "Show changed files for reviewing")
	verbose := flag.Bool("verbose", false, "Show progress and errors information")
	force := flag.Bool("force", false, "Continue processing despite checks or errors")

	flag.Parse()

	r := gr.Reviewer{*showFiles, *verbose}

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
