# git-reviewer

`git-reviewer` helps you find the best reviewer candidates for your branch
based on collaborators with the most experience across the files you changed.

## Usage

```
Usage of git-reviewer:
  -force=false: Continue processing despite checks or errors
  -ignore-extension="": Exclude changed paths that end with these extensions
     (--ignore-extension svg,png,jpg)
  -ignore-path="": Exclude file or files under path
     (--ignore-path main.go,src)
  -only-extension="": Only consider changed paths that end with one of these extensions
     (--only-extension go,js)
  -only-path="": Only consider file or files under path
     (--only-path main.go,src)
  -show-files=false: Show changed files for reviewing
  -since="": Consider commits after date when finding reviewers. Defaults to 6 months ago
     (format 'YYYY-MM-DD')
  -verbose=false: Show progress and errors information
  -version=false: Print the program version and exit
```

## Installing

If you have Go install:

* Ensure`$GOPATH/bin` is on your `PATH` (very likely if you have Go installed)
* `go get github.com/thedahv/git-reviewer`

If you don't, head over to the [releases](https://github.com/TheDahv/git-reviewer/releases)
page to find a binary for your system. Until I improve my build tools, I will only have
binaries for OSX and Linux.
