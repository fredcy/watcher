package main

import (
	"flag"
	"github.com/fredcy/watcher"
	"log"
	"regexp"
	"time"
)

func main() {
	var commandflag = flag.String("command", "", "command to run")
	var nostamp = flag.Bool("nostamp", false, "no datetime stamp for log output")
	var latency = flag.Duration("latency", time.Second, "seconds to wait for notifications to settle")
	var excludeflag = flag.String("exclude", "", "pattern of files to ignore")

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

	done := make(chan bool)
	watcher.Watchdirs(directories, &watcher.Options{command, *latency, exclude}, done)
}
