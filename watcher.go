package main

import (
	"flag"
	"log"
	"github.com/howeyc/fsnotify"
	"os/exec"
)

var command = flag.String("command", "/bin/ls", "command to run")
var directory = flag.String("directory", "/var/tmp", "directory to watch for changes")

func modified(filename string) {
	log.Printf("modified(%v)", filename)
	return
	out, err := exec.Command(*command, filename).CombinedOutput()
	if err != nil {
		log.Printf("command failed: %v\n%s", err, out)
		return
	}
	log.Printf("output is %s", out)
}

func main() {
	flag.Parse()
	log.Printf("watching %v", *directory)
	log.Printf("to run %v", *command)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	done := make(chan bool)

   // Process events
    go func() {
        for {
            select {
            case ev := <-watcher.Event:
                log.Println("event:", ev)
				if ev.IsModify() {
					modified(ev.Name)
				}
            case err := <-watcher.Error:
                log.Println("error:", err)
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
