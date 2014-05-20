package watcher

import (
	"flag"
	"fmt"
	"log"
	"github.com/howeyc/fsnotify"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Filename string
type Command string

var Debug = flag.Bool("debug", false, "print debug output")
var verbose = flag.Bool("verbose", false, "print verbose output")
var dryrun = flag.Bool("dryrun", false, "do not execute command")

type Options struct {
	Command Command
	Latency time.Duration
	Exclude *regexp.Regexp
}

type changehandler func(Filename)

// make_filechan calls the handler on the filename when there is at
// least one input to the channel (that it returns) followed by quiet period.
func make_filechan(filename Filename, latency time.Duration, handler changehandler) chan<- bool {
	if *Debug { log.Printf("make_filechan(%v)", filename) }
	c := make(chan bool)
	go func() {
		timer := time.NewTimer(time.Second)
		timer.Stop()
		for {
			select {
			case <-c:
				if *Debug { log.Printf("make_filechan(%v) pinged", filename) }
				timer.Reset(latency)
			case <-timer.C:
				handler(filename)
			}
		}
	}()
	return c
}

// Watchdirs waits for changes (including creations) to files in the
// given directories and handles them when they change.
func Watchdirs(directories []string, opts *Options, done chan bool) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Error: fsnotify.NewWatcher: ", err)
	}

	var handler changehandler
	if opts.Command != "" {
		handler = func(filename Filename) {
			filenames := []Filename{filename}
			handle(filenames, opts.Command)
		}
	} else {
		handler = func(filename Filename) {
			fmt.Println(string(filename))
		}
	}		

	// Read events from the fsnotify watcher and for interesting
	// events write to the channel created for each unique filename.
    go func() {
		filechans := make(map[Filename]chan<- bool)
        for {
            select {
            case ev := <-watcher.Event:
                if *Debug { log.Println("from watcher.Event:", ev) }
				if ev.IsCreate() || ev.IsModify() {
					if opts.Exclude != nil && opts.Exclude.Match([]byte(ev.Name)) {
						if *Debug { log.Println("Excluding:", ev.Name) }
					} else {
						filename := Filename(ev.Name)
						filechan, ok := filechans[filename]
						if ! ok {
							filechan = make_filechan(filename, opts.Latency, handler)
							filechans[filename] = filechan
						}
						filechan <- true
					}
				}
            case err := <-watcher.Error:
                log.Println("Error: watcher.Error:", err)
            }
        }
    }()

	for _, directory := range directories {
		log.Printf("Watching %v", directory)
		err = watcher.Watch(directory)
		if err != nil {
			log.Fatalf("Error: watcher.Watch(%s): %s", directory, err)
		}
	}

    <-done
    watcher.Close()
}

// handle runs the given shell command on the array of filenames.
// stdout and stderr of the command is combined and written to the
// process stdout. Any error return is logged.
func handle(filenames []Filename, command Command) {
	args := strings.Split(string(command), " ")
	for _, filename := range filenames {
		args = append(args, string(filename))
	}
	if *verbose {
		log.Printf("About to run command: %v", strings.Join(args, ""))
	}
	if ! *dryrun {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		fmt.Printf("%s", out)
		if err != nil {
			log.Printf("Error: Command failed: args=%v err='%v'", args, err)
		}
	}
}
