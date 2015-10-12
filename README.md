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
```

## Installing

If you have Go install:

* Ensure`$GOPATH/bin` is on your `PATH` (very likely if you have Go installed)
* Install `libgit2` on your machine (see "Contributing")
* `go get github.com/thedahv/git-reviewer`

If you don't, then you will be quite pleased when I upload binaries for your
system, but I haven't done that yet.

## Contributing

This project makes use of
[`github.com/libgit2/git2go`](https://github.com/libgit2/git2go) and
[`libgit2`](https://libgit2.github.com/). You'll need to install it on your
machine before making contributions.

### Ubuntu

* Install required packages:  
    `sudo apt-get update && sudo apt-get install git pkg-config cmake`
* [Download](https://github.com/libgit2/libgit2/releases) a copy of the project source
* Extract the source and enter the directory
* Build and install
    * `mkdir -p build && cd build`
    * `cmake .. -DCMAKE_INSTALL_PREFIX=/usr`
    * `cmake --build .`
    * `sudo cmake --build . --target install`
* Make sure Go is installed and `GOPATH` is configured. Example setup:
    * `wget https://storage.googleapis.com/golang/go1.5.1.linux-amd64.tar.gz`
    * `sudo tar -C /usr/local -xzf go1.5.1.linux-amd64.tar.gz`
    * `echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile`
    * `echo 'export GOPATH=~' >> .profile`
    * `source ~/.profile`
* Download and install `git2go`: `go get github.com/libgit2/git2go`

### OSX
If you are on OSX, you will need to add some system dependencies to support
installation. This is best accomplished with [Homebrew](http://brew.sh/):

* `brew install libgit2`
* `brew install pkg-config`
* `go get github.com/libgit2/git2go`
