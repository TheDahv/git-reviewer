# git-reviewer

`git-reviewer` helps you find the best reviewer candidates for your branch
based on collaborators with the most experience across the files you changed.

## Usage

```
Usage of git-reviewer:
  -force=false: Continue processing despite checks or errors
  -show-files=false: Show changed files for reviewing
  -since="": Consider commits after date when finding reviewers. Defaults to 6 months ago (format 'YYYY-MM-DD')
  -verbose=false: Show progress and errors information
```

## Installing

If you have Go install:

* Ensure`$GOPATH/bin` is on your `PATH` (very likely if you have Go installed)
* `go get github.com/thedahv/git-reviewer`

If you don't, then you will be quite pleased when I upload binaries for your
system, but I haven't done that yet.
