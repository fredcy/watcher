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
	"io"
	"log"
	"code.google.com/p/go.exp/fsnotify"
	"os"
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
	Filename Filename
	Timestamp time.Time
	Mask EventMask
	Fileinfo os.FileInfo
	Settled bool
}

type Filename string

// Debug controls debug output messages to stdout.
var Debug = flag.Bool("debug", false, "print debug output")

// Type Options controls the reporting and consolidation. The Latency
// is how long a file must be unchanged before we report the prior
// changes; similarly, it's also how long we wait to accumulate
// changes to multiple files. Subdirs controls whether we watch
// subdirectories after they are created.
type Options struct {
	Latency time.Duration
	Exclude *regexp.Regexp
	Subdirs bool
	Longform bool
	Group bool
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

// Watchdirs() is the main entry point for watching a list of directories.
func Watchdirs(directories []string, opts *Options, quit chan bool, out io.Writer) {
	events := watch(directories, opts, quit)
	if opts.Latency != 0 {
		events = simplify(events, opts)
	}
	if opts.Group {
		events = group(events, opts)
		for event := range events {
			fmt.Fprint(out, event.Filename)
			if event.Settled {
				fmt.Fprint(out, "\n")
			} else {
				fmt.Fprint(out, "\t")
			}
		}
		return
	}
	for event := range events {
		if opts.Longform {
			timestamp := event.Timestamp.Format("2006-01-02 15:04:05.999")
			info := ""
			if event.Fileinfo != nil {
				info = fmt.Sprintf("%d", event.Fileinfo.Size())
			}
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", timestamp, event.Filename, event.Mask, info)
		} else {
			fmt.Fprintln(out, event.Filename)
		}
	}
}


// simplify() reads a channel of events and writes a consolidated
// version of those events to its output channel. When there are
// multiple events on the same filename in quick succession only the
// last is passed on.
func simplify(events chan Event, opts *Options) chan Event {
	out := make(chan Event)
	done := make(chan bool)
	go func() {
		handlers := make(map[Filename]chan<-Event)
		for event := range events {
			handler, ok := handlers[event.Filename]
			if ! ok {
				handler = make_handler(event.Filename, opts.Latency, out, done)
				handlers[event.Filename] = handler
			}
			handler <- event
		}
		for filename := range handlers {
			close(handlers[filename])
			<- done
		}
		close(out)
	}()
	return out
}

// make_handler reads a channel of events for a single filename and
// writes an event to its output channel only after a latency period
// expires with no further events.
func make_handler(filename Filename, latency time.Duration, out chan Event, done chan bool) chan<- Event {
	if *Debug { log.Printf("make_handler(%v)", filename) }
	input := make(chan Event)
	go func() {
		var event Event
		var ok bool
		timer := time.NewTimer(latency)
		for {
			select {
			case event, ok = <-input:
				if ! ok {
					done <- true
					return
				}
				timer.Reset(latency)
			case <- timer.C:
				out <- event
			}
		}
	}()
	return input	
}

// group modifies a channel of Events, effectively grouping them by
// marking each one that appears last in a sequence before a latency
// period
func group(events chan Event, opts *Options) chan Event {
	out := make(chan Event)
	go func() {
		timer := time.NewTimer(opts.Latency)
		timer.Stop()
		active := true
		var eventprior Event
		haveprior := false
		for active {
			select {
			case event, ok := <-events:
				if ok {
					if haveprior {
						out <- eventprior
					}
					eventprior = event
					haveprior = true
					timer.Reset(opts.Latency)
				} else {
					active = false
				}
			case <-timer.C:
				eventprior.Settled = true
				out <- eventprior
				haveprior = false
			}
		}
		close(out)
	}()
	return out
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
				event.Filename = Filename(ev.Name)
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
		if *Debug { log.Println("watch() closing") }
		watcher.Close()
		close(events)
	}()
	return events
}
