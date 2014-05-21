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
	for _, directory := range directories {
		log.Printf("Watching %v", directory)
		err = watcher.Watch(directory)
		if err != nil {
			log.Fatalf("Error: watcher.Watch(%s): %s", directory, err)
		}
	}
	handler := make_accumulator(opts.Latency, opts.Command)

	// Read events from the fsnotify watcher and for interesting
	// events write to the channel created for each unique filename.
	filechans := make(map[Filename]chan<- bool)
	for {
		select {
		case ev := <-watcher.Event:
			if ev == nil {
				log.Println("nil event received")
				continue
			}
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
		case <-done:
			break
		}
	}
    watcher.Close()
}

// make_accumulator returns a function that is called when a filename
// has changed and which accumulates the reported filenames and when
// there is quiet period handles those filenames.
func make_accumulator(latency time.Duration, command Command) changehandler {
	is_changed := make(map[Filename]bool)
	filenames := make([]Filename, 0)
	timer := time.NewTimer(time.Second)
	timer.Stop()
	go func() {
		for {
			select {
			case <-timer.C:
				snames := make([]string, len(filenames))
				for i := range filenames {
					snames[i] = string(filenames[i])
				}
				fmt.Println(strings.Join(snames, "\t"))
				if command != "" {
					handle(filenames, command)
				}
				is_changed = make(map[Filename]bool)
				filenames = make([]Filename, 0)
			}
		}
	}()
	return func(filename Filename) {
		if *Debug { log.Printf("accumulator func(%v)", filename) }
		_, ok := is_changed[filename]
		if ! ok {
			is_changed[filename] = true
			filenames = append(filenames, filename)
		}
		timer.Reset(latency)
	}
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
