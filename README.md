watcher: command line utility reporting file system notifications
=======

This package, built on the https://github.com/howeyc/fsnotify package, provides a command line
application that watches multiple directories and/or files for changes
and reports those changes by printing the file names on standard output.

When a file is being changed frequently, such as a file being written
by a download, watcher waits for the file to become quiescent before
reporting the change. The latency interval is a command line option.

Watcher also adds the same latency to report multiple files at once
that have changed in the same latency period.
