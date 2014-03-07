package main

import (
	"flag"
	"fmt"
	"log"
	"github.com/howeyc/fsnotify"
	"os"
	"os/exec"
	"strings"
	"time"
)

type filename string

var command = flag.String("command", "/bin/ls", "command to run")
var directory = flag.String("directory", "/var/tmp", "directory to watch for changes")
var debug = flag.Bool("debug", false, "print debug output")
var verbose = flag.Bool("verbose", false, "print verbose output")
var dryrun = flag.Bool("dryrun", false, "do not execute command")
var nostamp = flag.Bool("nostamp", false, "no datetime stamp for log output")

var latency = 2

func handle(filename string) {
	if *verbose {
		log.Printf("About to run command: %s %s", *command, filename)
	}
	if ! *dryrun {
		args := append(strings.Split(*command, " "), filename)
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			log.Printf("Command failed: %v\n%s", err, out)
			return
		}
		log.Printf("Output is %s", out)
	}
}

func main() {
	log.SetOutput(os.Stderr)
	flag.Parse()
	if *nostamp {
		log.SetFlags(0)
	}
	log.Printf("Watching %v to run command '%v'", *directory, *command)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	done := make(chan bool)
	modified := make(chan string)

	// Read events from the watcher, passing on only Modify events to
	// the 'modified' channel.
    go func() {
        for {
            select {
            case ev := <-watcher.Event:
                if *debug { log.Println("event:", ev) }
				if ev.IsModify() {
					modified <- ev.Name
				}
            case err := <-watcher.Error:
                log.Println("Error:", err)
            }
        }
    }()

	// Act on file-modification events, adding a latency so that we
	// don't act until there are no events for a given time period.
	go func() {
		timer := time.NewTimer(time.Second)
		timer.Stop()
		var filename string
		for {
			select {
			case filename = <-modified:
				if *debug { log.Println("modified event: ", filename) }
				timer.Reset(time.Duration(latency) * time.Second)
			case <-timer.C:
				fmt.Println(filename)
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
