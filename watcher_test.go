package watcher

import (
	"bytes"
	"path/filepath"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"runtime"
	"testing"
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

func tempMkdir(t *testing.T) string {
    dir, err := ioutil.TempDir("", "watcher")
    if err != nil {
        t.Fatalf("failed to create test directory: %s", err)
    }
    return dir
}

func must_equal(t *testing.T, expected, got interface{}) {
	if expected != got {
		t.Errorf("Expected <<%v>>, got <<%v>>", expected, got)
	}
}

func TestTouch(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	testdir := tempMkdir(t)
	defer os.RemoveAll(testdir)

	dirs := []string{testdir}
	quit := make(chan bool)
	done := make(chan bool)
	var opts Options
	var out bytes.Buffer
	go func() {
		Watchdirs(dirs, &opts, quit, &out)
		done <- true
	}()

	time.Sleep(200 * time.Millisecond)
	testfile := filepath.Join(testdir, "foo")
	touch(testfile)
	time.Sleep(200 * time.Millisecond)
	quit <- true
	<- done
	must_equal(t, testfile + "\n", out.String())
}

func TestWatchdirs(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	testdir := tempMkdir(t)
	defer os.RemoveAll(testdir)
	dirs := []string{testdir}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	if os.PathSeparator == '\\' {
		opts.Exclude = regexp.MustCompile(`\\x[^\\]*$`)
	} else {
		opts.Exclude = regexp.MustCompile(`/x[^/]*$`)
	}
	opts.Group = true
	quit := make(chan bool)
	done := make(chan bool)
	var out bytes.Buffer
	go func() {
		Watchdirs(dirs, &opts, quit, &out)
		done <- true
	}()

	time.Sleep(100 * time.Millisecond) // allow Watchdirs to set up
	foo := filepath.Join(testdir, "foo")
	bar := filepath.Join(testdir, "bar")
	xfoo := filepath.Join(testdir, "xfoo")
	blah := filepath.Join(testdir, "blah")
	touch(foo)
	touch(bar)
	time.Sleep(opts.Latency / 2) // wait less than latency and touch again
	touch(bar)
	time.Sleep(3 * opts.Latency) // allow latency to expire twice
	touch(xfoo)
	// Pause briefly, otherwise fsnotify will sometimes miss the
	// following event on Mac OS X (a bug in fsnotify).
	time.Sleep(time.Millisecond)
	touch(blah)
	time.Sleep(3 * opts.Latency)
	quit <- true
	<- done

	must_equal(t, foo + "\t" + bar + "\n" + blah + "\n", out.String())
}

func TestSubdirs(t *testing.T) {
	testdir := tempMkdir(t)
	defer os.RemoveAll(testdir)
	dirs := []string{testdir + ""}
	var opts Options
	opts.Latency = 200 * time.Millisecond
	opts.Subdirs = true
	opts.Group = true
	quit := make(chan bool)
	done := make(chan bool)
	var out bytes.Buffer
	go func() {
		Watchdirs(dirs, &opts, quit, &out)
		done <- true
	}()
	
	subdir := filepath.Join(testdir, "subdir")
	subfile1 := filepath.Join(subdir, "one")
	subfile2 := filepath.Join(subdir, "two")
	time.Sleep(100 * time.Millisecond) // allow Watchdirs to set up
	mkdir(subdir)
	time.Sleep(opts.Latency / 2) // enough time for subdir watch to establish, but less than latency
	touch(subfile1)
	time.Sleep(3 * opts.Latency) // enough time for latency to expire twice
	touch(subfile2)
	time.Sleep(3 * opts.Latency) // enough time for latency to expire twice
	quit <- true
	<-done

	if runtime.GOOS == "windows" {
		// in Windows, touching a file causes the directory to be reported as MODIFIED as well
		must_equal(t, subfile1 + "\t" + subdir + "\n" + subfile2 + "\t" + subdir + "\n", out.String())
	} else {
		must_equal(t, subdir + "\t" + subfile1 + "\n" + subfile2 + "\n", out.String())
	}
}
