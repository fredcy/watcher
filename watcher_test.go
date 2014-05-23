package watcher

import (
	"log"
	"os"
	"regexp"
	"time"
)

func touch(filename string) {
	fp, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	fp.Close()
}

func mkdir(dirname string) {
	perm := os.FileMode(0750)
	err := os.Mkdir(dirname, perm)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleWatchdirs() {
	dirs := []string{"/tmp", "/var/tmp"}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	opts.Exclude = regexp.MustCompile("/x[^/]*$")
	done := make(chan bool)
	go func() {
		Watchdirs(dirs, &opts, done)
	}()

	time.Sleep(200 * time.Millisecond) // allow Watchdirs to set up
	touch("/var/tmp/foo")
	touch("/var/tmp/bar")
	time.Sleep(3 * opts.Latency)		// allow latency to expire twice
	touch("/var/tmp/xfoo")
	touch("/var/tmp/blah")
	time.Sleep(3 * opts.Latency)
	done <- true

	// Output:
	// /var/tmp/foo	/var/tmp/bar
	// /var/tmp/blah
}

func ExampleSubdirs() {
	dirs := []string{"/var/tmp"}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	opts.Subdirs = true
	done := make(chan bool)
	go func() {
		Watchdirs(dirs, &opts, done)
	}()
	
	time.Sleep(200 * time.Millisecond) // allow Watchdirs to set up
	mkdir("/var/tmp/subdirtest")
	time.Sleep(opts.Latency / 2) // enough time for subdir watch to establish, but less than latency
	touch("/var/tmp/subdirtest/one")
	time.Sleep(3 * opts.Latency) // enough time for latency to expire twice
	done <- true
	if err := os.RemoveAll("/var/tmp/subdirtest"); err != nil {
		log.Fatal(err)
	}

	// Output:
	// /var/tmp/subdirtest	/var/tmp/subdirtest/one
}
