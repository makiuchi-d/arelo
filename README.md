# Arelo - a simple auto reload utility

[![go test](https://github.com/makiuchi-d/arelo/actions/workflows/test.yml/badge.svg)](https://github.com/makiuchi-d/arelo/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/makiuchi-d/arelo)](https://goreportcard.com/report/github.com/makiuchi-d/arelo)

Arelo executes the specified command and monitors the files under the target directory.
When the file that matches the pattern has been modified, restart the command.

## Features

 - Simple command line interface without config file
 - Monitoring file patterns are specified as glob
   - globstar (**; matches to zero or more directories) supported
   - can match the no extention filename
   - can match the hidden filename which starts with "."
 - Safely terminate child processes
 - Any command line tool can be executed
   - not only go project
   - can execute shell script
 - No unnesesary servers
   - no need to use local port unlike http server

## Install

```
go install github.com/makiuchi-d/arelo@latest
```

Or, you can download the executable binaries from the [release page](https://github.com/makiuchi-d/arelo/releases).

To get a static-linked executable binary, build with `CGO_ENABLED=0`.

## Quick start

Run this command in your Go project directory.

```
arelo -p '**/*.go' -i '**/.*' -i '**/*_test.go' -- go run .
```

## Usage

```
Usage: arelo [OPTION]... -- COMMAND
Run the COMMAND and restart when a file matches the pattern has been modified.

Options:
  -d, --delay duration   duration to delay the restart of the command (default 1s)
  -f, --filter event     filter file system event (CREATE|WRITE|REMOVE|RENAME|CHMOD)
  -h, --help             display this message
  -i, --ignore glob      ignore pathname glob pattern
  -p, --pattern glob     trigger pathname glob pattern (default "**")
  -r, --restart          restart the command on exit
  -s, --signal signal    signal used to stop the command (default "SIGTERM")
  -t, --target path      observation target path (default "./")
  -v, --verbose          verbose output
  -V, --version          display version
```

### Options

#### -t, --target path

Monitor file modifications under the `path` directory.
The subdirectories are also monitored unless they match to the ignore patterns.

This option can be set multiple times.

The default value is the current directory ("./").

Note:
This option can be file instead of directory, 
but arelo cannot follow modification after the file has been removed/renamed.

#### -p, --pattern glob

Restart command when the modified file is matched to this pattern.

The pattern is specified as an extended glob
that supports `{alt1,...}`, `**` like zsh or bash with globstar option.
And note that the path delimiter is `/` even on Windows.

This option can set multiple times.

The default value ("**") is a pattern that matches any file in the target directories and their subdirectories.

#### -i, --ignore glob

Ignore the file or directory whose names is matched to this pattern.

This option takes precedence over the --pattern option.

This option can set multiple times.


#### -f, --filter event

Filter the filesystem event to ignore it.

The event can be `CREATE`, `WRITE`, `REMOVE`, `RENAME` or `CHMOD`.

This option can set multiple times.

#### -d, --delay duration

Delay the restart of the command from the detection of the pattern matched file modification.
The detections within the delay are ignored.

The duration is specified as a number with a unit suffix ("ns", "us" (or "µs"), "ms", "s", "m", "h").

#### -s, --signal signal

This signal will be sent to stop the command on restart.
The default signal is `SIGTERM`.

This option can be `SIGHUP`, `SIGINT`, `SIGQUIT`, `SIGKILL`, `SIGUSR1`, `SIGUSR2`, `SIGWINCH` or `SIGTERM`.

This option is not available on Windows.

#### -r, --restart

Automatically restart the command when it exits, similar to when the pattern matched file is modified.

#### -v, --verbose

Output logs verbosely.

#### -V, --version

Print version informatin.

#### -h, --help

Print usage.

### Example

```
arelo -t ./src -t ./html -p '**/*.{go,html,yaml}' -i '**/.*' -- go run .
```

####  `-t ./src -t ./html`

Monitor files under the ./src or ./html directories.

#### `-p '**/*.{go,html,yaml}'`

Restart command when any *.go, *.html, *.yml file under the target, sub, and subsub... directories modified.

#### `-i '**/.*'`

Ignore files/directories whose name starts with '.'.

#### `go run .`

Command to run.

## Similar projects

 - [realize](https://github.com/oxequa/realize)
 - [fresh](https://github.com/gravityblast/fresh)
 - [gin](https://github.com/codegangsta/gin)
 - [go-task](https://github.com/go-task/task)
 - [air](https://github.com/cosmtrek/air)
 - [reflex](https://github.com/cespare/reflex)
