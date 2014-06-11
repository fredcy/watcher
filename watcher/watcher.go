// Package main provides a command line main() function that calls
// watcher.Watchdirs()
package main

import (
	"path/filepath"
	"flag"
	"github.com/fredcy/watcher"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

func main() {
	var nostamp = flag.Bool("nostamp", false, "no datetime stamp for log output")
	var latency = flag.Duration("latency", 200 * time.Millisecond, "seconds to wait for notifications to settle")
	var excludeflag = flag.String("exclude", "", "pattern of files to ignore")
	var subdirflag = flag.Bool("subdirs", false, "watch subdirectories too")
	var longflag = flag.Bool("long", false, "long format outout")
	var groupflag = flag.Bool("group", false, "group files changed before latency period")

	flag.Parse()
	if *nostamp {
		log.SetFlags(0)
	} else {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	}
	var directories = flag.Args()
	var exclude *regexp.Regexp
	if *excludeflag != "" {
		exclude = regexp.MustCompile(*excludeflag)
	}
	var dirstowatch []string
	if *subdirflag {
		subdirs := make([]string, 0)
		badfiles := make(map[string]bool)
		walkfn := func(path string, info os.FileInfo, err error) error {
			//log.Printf("walkfn(%v, %v, %v)", path, info, err)
			switch {
			case err != nil:
				log.Printf("warning: %v", err)
				switch {
				case strings.Contains(err.Error(), "no such file or directory"):
					// handle first since info.IsDir() cannot work in this case
					badfiles[path] = true
				case info.IsDir():
					// directories sometimes get visited twice (oddly)
					// with an error on the second visit only
					badfiles[path] = true
					return filepath.SkipDir
				}
			case info.IsDir():
				if exclude != nil && exclude.MatchString(path) {
					if *watcher.Debug { log.Printf("Excluding %s", path) }
					return filepath.SkipDir
				}
				subdirs = append(subdirs, path)
			case info.Mode() & os.ModeNamedPipe == os.ModeNamedPipe:
				dirpath := filepath.Dir(path)
				log.Printf("Warning: %s is a named pipe; ignoring %s",
					path, dirpath)
				badfiles[dirpath] = true
			}
			return nil
		}
		for _, directory := range(directories) {
			filepath.Walk(directory, walkfn)
		}
		// filter the generated list of directories, removing any marked as bad above
		for _, dir := range(subdirs) {
			if ! badfiles[dir] {
				dirstowatch = append(dirstowatch, dir)
			}
		}
	} else {
		dirstowatch = directories
	}

	done := make(chan bool)
	opts := watcher.Options{*latency, exclude, *subdirflag, *longflag, *groupflag}
	watcher.Watchdirs(dirstowatch, &opts, done, os.Stdout)
}
