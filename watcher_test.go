package watcher

import (
	"log"
	"os"
	"regexp"
	"time"
)

func touch(filename string) {
	if *Debug { log.Printf("touch(%v)", filename) }
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
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	testdir := "tmp-test"
	mkdir(testdir)
	dirs := []string{testdir}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	opts.Exclude = regexp.MustCompile("/x[^/]*$")
	quit := make(chan bool)
	done := make(chan bool)
	go func() {
		Watchdirs(dirs, &opts, quit)
		done <- true
	}()

	time.Sleep(100 * time.Millisecond) // allow Watchdirs to set up
	touch(testdir + "/foo")
	touch(testdir + "/bar")
	time.Sleep(opts.Latency / 2) // wait less than latency and touch again
	touch(testdir + "/bar")
	time.Sleep(3 * opts.Latency) // allow latency to expire twice
	touch(testdir + "/xfoo")
	touch(testdir + "/blah")
	time.Sleep(3 * opts.Latency)
	quit <- true
	<- done
	if err := os.RemoveAll(testdir); err != nil {
		log.Fatal(err)
	}

	// Output:
	// tmp-test/foo	tmp-test/bar
	// tmp-test/blah
}

func ExampleSubdirs() {
	testdir := "tmp-test"
	mkdir(testdir)
	dirs := []string{testdir + ""}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	opts.Subdirs = true
	quit := make(chan bool)
	done := make(chan bool)
	go func() {
		Watchdirs(dirs, &opts, quit)
		done <- true
	}()
	
	time.Sleep(100 * time.Millisecond) // allow Watchdirs to set up
	mkdir(testdir + "/subdirtest")
	time.Sleep(opts.Latency / 2) // enough time for subdir watch to establish, but less than latency
	touch(testdir + "/subdirtest/one")
	time.Sleep(3 * opts.Latency) // enough time for latency to expire twice
	touch(testdir + "/subdirtest/two")
	time.Sleep(3 * opts.Latency) // enough time for latency to expire twice
	quit <- true
	<-done
	if err := os.RemoveAll(testdir); err != nil {
		log.Fatal(err)
	}

	// Output:
	// tmp-test/subdirtest	tmp-test/subdirtest/one
	// tmp-test/subdirtest/two
}
