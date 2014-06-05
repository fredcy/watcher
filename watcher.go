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
//	"github.com/howeyc/fsnotify"
	"code.google.com/p/go.exp/fsnotify"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type EventMask uint32
func (mask EventMask) String() string {
	var strs []string
	if mask & EventCreate == EventCreate {
		strs = append(strs, "CREATE")
	}
	if mask & EventModify == EventModify {
		strs = append(strs, "MODIFY")
	}
	if mask & EventDelete == EventDelete {
		strs = append(strs, "DELETE")
	}
	if mask & EventRename == EventRename {
		strs = append(strs, "RENAME")
	}
	if mask & EventAttrib == EventAttrib {
		strs = append(strs, "ATTRIB")
	}
	return strings.Join(strs, "|")
}

const (
	EventCreate  EventMask = 1 << iota
	EventModify
	EventRename
	EventDelete
	EventAttrib
)

type Event struct {
	Filename string
	Timestamp time.Time
	Mask EventMask
	Fileinfo os.FileInfo
}

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
// changes to multiple files. Subdirs controls whether we watch
// subdirectories after they are created.
type Options struct {
	Command Command
	Latency time.Duration
	Exclude *regexp.Regexp
	Subdirs bool
	Longform bool
}

// make_filechan runs a goroutine that watches a channel (that it
// returns) indicating changes to the particular file.  When there are
// no further changes for the latency period it reports to the
// 'changed' channel.
func make_filechan(filename Filename, latency time.Duration, changed chan Filename, done chan bool) chan<- bool {
	if *Debug { log.Printf("make_filechan(%v)", filename) }
	c := make(chan bool)
	go func() {
		timer := time.NewTimer(latency)
		for {
			select {
			case _, ok := <-c:
				if ! ok {
					if *Debug { log.Printf("Stopping handler for %v", filename) }
					done <- true
					return
				}
				if *Debug { log.Printf("make_filechan(%v) pinged", filename) }
				timer.Reset(latency)
			case <-timer.C:
				changed <- filename
			}
		}
	}()
	return c
}

// Watchdirs waits for changes (including creations) to files in the
// given directories and handles them when they change. Default
// handling is just to write the names to stdout.  If a command is
// provided in the options it is also run.
func Watchdirs(directories []string, opts *Options, quit chan bool) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Error: fsnotify.NewWatcher: ", err)
	}
	for _, directory := range directories {
		if *Debug { log.Printf("Watching %v", directory) }
		err = watcher.Watch(directory)
		if err != nil {
			log.Printf("Error: watcher.Watch(%s): %s", directory, err)
			if strings.Contains(err.Error(), "too many open files") {
				log.Fatal("quitting")
			}
		}
	}
	changed := make_accumulator(opts)

	// Read events from the fsnotify watcher and for interesting
	// events write to the channel created for each unique filename.
	filechans := make(map[Filename]chan<- bool)
	done := make(chan bool)		// for filechans to signal when they are done
	active := true
	for active {
		select {
		case ev, ok := <-watcher.Event:
			if ! ok {
				log.Fatal("watcher.Event channel closed unexpectedly")
			}
			if *Debug { log.Println("from watcher.Event:", ev) }
			if ev.IsCreate() || ev.IsModify() {
				if opts.Exclude != nil && opts.Exclude.Match([]byte(ev.Name)) {
					if *Debug { log.Println("Excluding:", ev.Name) }
				} else {
					if opts.Subdirs && isdir(ev.Name) {
						watcher.Watch(ev.Name)
						if *Debug { log.Printf("Adding watch of %v", ev.Name) }
					}
					filename := Filename(ev.Name)
					filechan, ok := filechans[filename]
					if ok {
						filechan <- true
					} else {
						filechan = make_filechan(filename, opts.Latency, changed, done)
						filechans[filename] = filechan
					}
				}
			}
		case err := <-watcher.Error:
			log.Println("Error: watcher.Error:", err)
		case <-quit:
			if *Debug { log.Printf("Stopping main Watcher loop") }
			for filename := range filechans {
				close(filechans[filename])
				<- done
			}
			close(changed)
			active = false
		}
	}
    watcher.Close()
	if *Debug { log.Printf("Watcher returning") }
}

// make_accumulator returns a channel that is written when a filename
// has changed and which accumulates the reported filenames and when
// there is quiet period handles those filenames.
func make_accumulator(opts *Options) chan Filename {
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
		if opts.Longform {
			timestamp := time.Now().Format("2006-01-02T15:04:05.999")
			fmt.Printf("%s\t%s\n", timestamp, strings.Join(snames, "\t"))
		} else {
			fmt.Println(strings.Join(snames, "\t"))
		}
		if opts.Command != "" {
			run_command(filenames, opts.Command)
		}
	}
	changed := make(chan Filename)
	go func() {
		reset_accum()
		for {
			select {
			case filename, ok := <-changed:
				if ! ok {
					if *Debug { log.Printf("Stopping accumulator") }
					return
				}
				if *Debug { log.Printf("accumulator: filename=%v", filename) }
				_, found := is_changed[filename]
				if ! found {
					is_changed[filename] = true
					filenames = append(filenames, filename)
				}
				timer.Reset(opts.Latency)
				
			case <-timer.C:
				// No more file changes during latency period, so
				// report what we've got
				report()
				reset_accum()
			}
		}
	}()
	return changed
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

func isdir(filename string) bool {
	fi, err := os.Stat(filename)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			if *Debug { log.Print(err) }
		} else {
			log.Panic(err)
		}
		return false
	}
	return fi.IsDir()
}

func WatchRaw(directories []string, opts *Options, quit chan bool) {
	events := watch(directories, opts, quit)
	for event := range events {
		timestamp := event.Timestamp.Format("2006-01-02 15:04:05.999")
		info := ""
		if event.Fileinfo != nil {
			info = fmt.Sprintf("%d", event.Fileinfo.Size())
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", timestamp, event.Filename, event.Mask, info)
	}
}


// watch returns a channel that produces Event items reporting file
// changes within the given directories. It wraps an fsnotify watcher
// so as to ignore events on filenames that match an pattern, to add
// a timestamp to the event data, to establish new watches as
// subdirectories are created, and to add fileinfo information.
func watch(directories []string, opts *Options, quit chan bool) chan Event {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Panic(err)
	}
	for _, directory := range directories {
		if *Debug { log.Printf("Watching %v", directory) }
		err = watcher.Watch(directory)
		if err != nil {
			log.Printf("Error: watcher.Watch(%s): %s\n", directory, err)
			if strings.Contains(err.Error(), "too many open files") {
				log.Panic("quitting")
			}
		}
	}
	if *Debug { log.Println("All directory watches established") }
	events := make(chan Event)
	go func() {
		active := true
		for active {
			select {
			case ev, ok := <-watcher.Event:
				if ! ok {
					log.Panic("watcher.Event channel closed unexpectedly")
				}
				var event Event
				event.Timestamp = time.Now() // record event time ASAP

				if *Debug { log.Println("from watcher.Event:", ev) }
				if opts.Exclude != nil && opts.Exclude.MatchString(ev.Name) {
					if *Debug { log.Println("Excluding:", ev.Name) }
					break
				}
				if opts.Subdirs && ev.IsCreate() && isdir(ev.Name) {
					watcher.Watch(ev.Name)
					if *Debug { log.Printf("Adding watch of %v", ev.Name) }
				}
				event.Filename = ev.Name
				if ev.IsCreate() { event.Mask |= EventCreate } 
				if ev.IsModify() { event.Mask |= EventModify } 
				if ev.IsRename() { event.Mask |= EventRename } 
				if ev.IsDelete() { event.Mask |= EventDelete } 
				if ev.IsAttrib() { event.Mask |= EventAttrib } 
				event.Fileinfo, err = os.Stat(ev.Name)
				if err != nil {
					if strings.Contains(err.Error(), "no such file or directory") {
						if *Debug {
							log.Print(err)
						}
						// ignore this error if not debugging
					} else {
						log.Print(err)
					}
				}
				events <- event
			case err := <-watcher.Error:
				log.Println("Error: watcher.Error", err)
			case <-quit:
				active = false
			}
		}
		watcher.Close()
		close(events)
	}()
	return events
}
