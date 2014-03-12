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

func ExampleWatchdirs() {
	dirs := []string{"/tmp", "/var/tmp"}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	opts.Exclude = regexp.MustCompile("/x[^/]*$")
	done := make(chan bool)
	//*Debug = true
	go func() {
		Watchdirs(dirs, &opts, done)
	}()

	time.Sleep(200 * time.Millisecond) // allow Watchdirs to set up
	touch("/var/tmp/foo")
	touch("/var/tmp/bar")
	time.Sleep(time.Second)		// must be longer than the latency set above
	touch("/var/tmp/xfoo")
	touch("/var/tmp/blah")
	time.Sleep(time.Second)
	done <- true

	// Output:
	// /var/tmp/foo	/var/tmp/bar
	// /var/tmp/blah
}
