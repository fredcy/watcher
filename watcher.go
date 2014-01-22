package main

import (
	"flag"
	"log"
	"github.com/howeyc/fsnotify"
	"os/exec"
	"time"
)

type filename string

var command = flag.String("command", "/bin/ls", "command to run")
var directory = flag.String("directory", "/var/tmp", "directory to watch for changes")
var debug = flag.Bool("debug", false, "print debug output")
var verbose = flag.Bool("verbose", false, "print verbose output")
var dryrun = flag.Bool("dryrun", false, "do not execute command")

func handle(filename string) {
	if *verbose {
		log.Printf("About to run command: %s %s", *command, filename)
	}
	if ! *dryrun {
		out, err := exec.Command(*command, filename).CombinedOutput()
		if err != nil {
			log.Printf("command failed: %v\n%s", err, out)
			return
		}
		log.Printf("output is %s", out)
	}
}

func main() {
	flag.Parse()
	log.Printf("watching %v\nto run %v", *directory, *command)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	done := make(chan bool)
	modified := make(chan string)

	// read events from the watcher
    go func() {
        for {
            select {
            case ev := <-watcher.Event:
                if *debug { log.Println("event:", ev) }
				if ev.IsModify() {
					modified <- ev.Name
				}
            case err := <-watcher.Error:
                log.Println("error:", err)
            }
        }
    }()

	go func() {
		timer := time.NewTimer(time.Second)
		timer.Stop()
		var filename string
		for {
			select {
			case filename = <-modified:
				if *debug { log.Println("modified event: ", filename) }
				timer.Reset(2 * time.Second)
			case <-timer.C:
				handle(filename)
			}
		}
	}()

    err = watcher.Watch(*directory)
    if err != nil {
        log.Fatal(err)
    }
    <-done
    watcher.Close()
}
