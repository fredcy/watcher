// Package watcher provides a function and command line utility that
// watches one or more files and directories and reports whenever they
// are changed, writing their names to stdout. It can also run a
// specified shell command on the changed files. It is built on top of
// the fsnotify package.
//
// The reporting does some consolidation of events so that changes are
// reported only when the file becomes quiescent. There is also
// consolidation of changes over multiple files.
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
// Command is the shell command to run when files change.
type Command string

// Debug controls debug output messages to stdout.
var Debug = flag.Bool("debug", false, "print debug output")
var verbose = flag.Bool("verbose", false, "print verbose output")
var dryrun = flag.Bool("dryrun", false, "do not execute command")

// Type Options controls the reporting and consolidation. The Command
// (if any) is run with the changed filenames as arguments. The
// Latency is how long a file must be unchanged before we report the
// prior changes; similarly, it's also how long we wait to accumulate
// changes to multiple files.
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
// given directories and handles them when they change. Default
// handling is just to write the names to stdout.  If a command is
// provided in the options it is also run.
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
	var is_changed map[Filename]bool
	var filenames []Filename
	timer := time.NewTimer(time.Second)
	timer.Stop()
	reset_accum := func() {
		is_changed = make(map[Filename]bool)
		filenames = make([]Filename, 0)
	}
	report := func() {
		snames := make([]string, len(filenames))
		for i := range filenames {
			snames[i] = string(filenames[i])
		}
		fmt.Println(strings.Join(snames, "\t"))
		if command != "" {
			run_command(filenames, command)
		}
	}
	go func() {
		reset_accum()
		for {
			select {
			case <-timer.C:
				// No more file changes during latency period, so
				// report what we've got
				report()
				reset_accum()
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

// run_command runs the given shell command on the array of filenames.
// stdout and stderr of the command is combined and written to the
// process stdout. Any error return is logged.
func run_command(filenames []Filename, command Command) {
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
