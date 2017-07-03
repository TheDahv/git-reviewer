package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"

	gr "github.com/thedahv/git-reviewer/src"
	gogit "gopkg.in/src-d/go-git.v4"
)

const version = "0.0.3"

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
	ie := flag.String("ignore-extension", "", "Exclude changed paths that end with"+
		" these extensions (--ignore-extension svg,png,jpg)")
	oe := flag.String("only-extension", "", "Only consider changed paths that end with"+
		" one of these extensions (--only-extension go,js)")
	ip := flag.String("ignore-path", "", "Exclude file or files under path"+
		" (--ignore-path main.go,src)")
	op := flag.String("only-path", "", "Only consider file or files under path"+
		" (--only-path main.go,src)")
	v := flag.Bool("version", false, "Print the program version and exit")

	flag.Parse()

	if *v {
		fmt.Printf("git-reviewer version %s\n", version)
		return
	}

	spaceOrComma := func(r rune) bool {
		switch r {
		case ' ', ',':
			return true
		}
		return false
	}

	ignoredExtensions := strings.FieldsFunc(*ie, spaceOrComma)
	onlyExtensions := strings.FieldsFunc(*oe, spaceOrComma)
	ignoredPaths := strings.FieldsFunc(*ip, spaceOrComma)
	onlyPaths := strings.FieldsFunc(*op, spaceOrComma)

	err := checkDateArg(*since)
	if len(*since) > 0 && err != nil {
		fmt.Println("Problem with input format for 'since' argument. Run 'git reviewer -h'")
		return
	}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Unable to open current directory: %v\n", err)
		return
	}

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		fmt.Printf("Unable to open repository: %v\n", err)
		return
	}

	r := gr.ContributionCounter{
		Repo:              repo,
		ShowFiles:         *showFiles,
		Verbose:           *verbose,
		Since:             *since,
		IgnoredExtensions: ignoredExtensions,
		OnlyExtensions:    onlyExtensions,
		IgnoredPaths:      ignoredPaths,
		OnlyPaths:         onlyPaths,
	}

	// TODO take mailmap paths from command args
	var mailmapPaths []string
	if u, err := user.Current(); err == nil {
		mailmapPaths = append(mailmapPaths, u.HomeDir+"/.mailmap")
	}
	if cwd, err := os.Getwd(); err == nil {
		mailmapPaths = append(mailmapPaths, cwd+"/.mailmap")
		mailmapPaths = append(mailmapPaths, cwd+"/mailmap")
	}
	r.BuildMailmap(mailmapPaths...)

	// Determine if branch is reviewable
	if behind, err := r.BranchBehind(); behind || err != nil {
		if err != nil {
			fmt.Printf("There was an error determining branch state: %v\n", err)
			return
		}

		fmt.Println("Current branch is behind master. Merge up!")
		if *force == false {
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
	//reviewers, err := r.FindReviewers(files)
	reviewers, err := r.FindReviewers(files)
	if err != nil {
		fmt.Printf("There was an error finding reviewers: %v\n", err)
		return
	}

	fmt.Println(reviewers)
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
