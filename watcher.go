package main

import (
	"flag"
	"fmt"
	"log"
	"github.com/howeyc/fsnotify"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Filename string
type Command string

var debug = flag.Bool("debug", false, "print debug output")
var verbose = flag.Bool("verbose", false, "print verbose output")
var dryrun = flag.Bool("dryrun", false, "do not execute command")

func main() {
	var commandflag = flag.String("command", "", "command to run")
	var nostamp = flag.Bool("nostamp", false, "no datetime stamp for log output")
	var latency = flag.Duration("latency", time.Second, "seconds to wait for notifications to settle")
	var excludeflag = flag.String("exclude", "", "pattern of files to ignore")

	log.SetOutput(os.Stderr)
	flag.Parse()
	if *nostamp {
		log.SetFlags(0)
	}
	var directories = flag.Args()
	var command = Command(*commandflag)
	if *debug { log.Printf("Command is \"%v\"", command) }

	var exclude *regexp.Regexp
	if *excludeflag != "" {
		exclude = regexp.MustCompile(*excludeflag)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Error: fsnotify.NewWatcher: ", err)
	}
	done := make(chan bool)
	modified := make(chan Filename)
	toreport := make(chan Filename, 100)

	// Read events from the watcher and for interesting events pass
	// the filename on to the 'modified' channel.
    go func() {
        for {
            select {
            case ev := <-watcher.Event:
                if *debug { log.Println("from watcher.Event:", ev) }
				if ev.IsCreate() || ev.IsModify() {
					if exclude != nil && exclude.Match([]byte(ev.Name)) {
						if *debug { log.Println("Excluding:", ev.Name) }
					} else {
						modified <- Filename(ev.Name)
					}
				}
            case err := <-watcher.Error:
                log.Println("Error: watcher.Error:", err)
            }
        }
    }()

	// Act on file-modification events, adding a latency so that we
	// don't act until there are no events for a given time period.
	go func() {
		timer := time.NewTimer(time.Second)
		timer.Stop()
		var filename Filename
		for {
			select {
			case filename = <-modified:
				if *debug { log.Println("from modified:", filename) }
				toreport <- filename
				timer.Reset(time.Duration(*latency))
			case <-timer.C:
				reportall(toreport, command)
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

// reportall reads all filenames from the channel (without blocking),
// removes duplicates, reports the filenames on one line of output,
// and runs the command (if any) on the filenames.
func reportall(toreport chan Filename, command Command) {
	names := getall(toreport)
	snames := make([]string, len(names))
	for i := range names {
		snames[i] = string(names[i])
	}
	fmt.Println(strings.Join(snames, "\t"))
	if command != "" {
		handle(names, command)
	}
}

// getall reads all the buffered filenames from the channel (without
// blocking) and returns an array of the unique values.
func getall(toreport chan Filename) []Filename {
	reported := make(map [Filename] bool)
	names := make([]Filename, 0)
	for {
		select {
		case filename := <-toreport:
			if ! reported[filename] {
				names = append(names, filename)
				reported[filename] = true
			}
		default:
			return names
		}
	}
}

// handle runs the given shell command on the array of filenames.
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
		if err != nil {
			log.Printf("Command failed: %v\n%s", err, out)
			return
		}
		log.Printf("Output is %s", out)
	}
}
