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
	"time"
)

func main() {
	var commandflag = flag.String("command", "", "command to run")
	var nostamp = flag.Bool("nostamp", false, "no datetime stamp for log output")
	var latency = flag.Duration("latency", time.Second, "seconds to wait for notifications to settle")
	var excludeflag = flag.String("exclude", "", "pattern of files to ignore")
	var subdirflag = flag.Bool("subdirs", false, "watch subdirectories too")

	flag.Parse()
	if *nostamp {
		log.SetFlags(0)
	}
	var directories = flag.Args()
	var command = watcher.Command(*commandflag)
	if *watcher.Debug { log.Printf("Command is \"%v\"", command) }

	var exclude *regexp.Regexp
	if *excludeflag != "" {
		exclude = regexp.MustCompile(*excludeflag)
	}

	if *subdirflag {
		subdirs := make([]string, 0)
		baddirs := make(map[string]bool)
		walkfn := func(path string, info os.FileInfo, err error) error {
			//log.Printf("walkfn(%v, %v, %v)", path, info, err)
			if err != nil {
				log.Printf("warning: %v", err)
				if info.IsDir() {
					baddirs[path] = true
				}
				return nil
			}
			if info.IsDir() {
				subdirs = append(subdirs, path)
			}
			return nil
		}
		for _, directory := range(directories) {
			filepath.Walk(directory, walkfn)
		}
		for _, dir := range(subdirs) {
			if ! baddirs[dir] {
				directories = append(directories, dir)
			}
		}
	}

	done := make(chan bool)
	watcher.Watchdirs(directories, &watcher.Options{command, *latency, exclude}, done)
}
