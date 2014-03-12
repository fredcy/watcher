package watcher

import (
	"log"
	"os"
	"time"
)

func ExampleWatchdirs() {
	dirs := []string{"/tmp", "/var/tmp"}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	done := make(chan bool)
	//*Debug = true
	go func() {
		Watchdirs(dirs, &opts, done)
	}()
	time.Sleep(200 * time.Millisecond) // allow Watchdirs to set up
	fp, err := os.Create("/var/tmp/foo")
	if err != nil {
		log.Fatal(err)
	}
	fp.Close()
	time.Sleep(time.Second)		// must be longer than the latency set above
	done <- true
	// Output:
	// /var/tmp/foo
}
