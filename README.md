# Arelo - a simple auto reload utility

Arelo executes the specified command and monitors the files under the target directory.
When the file that matches the pattern has been modified, restart the command.

## Features

 - Simple command line interface without config file
 - Monitoring file patterns are specified as glob
   - globstar (**; matches to zero or more directories) supported
   - can match the no extention filename
 - Any command line tool can be executed
   - not only go project
 - No http server

## Install

```
go get github.com/makiuchi-d/arelo
```

Or, you can download the executable binaries from the [release page](https://github.com/makiuchi-d/arelo/releases).

## Usage

```
Usage: arelo [OPTION]... -- COMMAND
Run the COMMAND and restart when a file matches the pattern has been modified.

Options:
  -d, --delay duration   duration to delay the restart of the command. (default 1s)
  -h, --help             show this document.
  -i, --ignore glob      ignore pathname glob pattern.
  -p, --pattern glob     trigger pathname glob pattern. (required)
  -s, --signal signal    signal to stop the command. (default "SIGTERM")
  -t, --target path      observation target path. (default "./")
  -v, --verbose          verbose output.
```

### Options

#### -t, --target path

Monitor file modifications under the `path` directory.
The subdirectories are also monitored unless they match to the ignore patterns.

This option can be set multiple times.

Note:
This option can be file instead of directory, 
but arelo cannot follow modification after the file has been removed/renamed.

#### -p, --pattern glob

Restart command when the modified file is matched to this pattern.
The pattern is specified as an extended glob
that supports `{alt1,...}`, `**` like zsh or bash with globstar option.

This option can set multiple times.

#### -i, --ignore glob

Ignore the file or directory whose names is matched to this pattern.

This option takes precedence over the --pattern option.

This option can set multiple times.


#### -d, --delay duration

Delay the restart of the command from the detection of the pattern matched file modification.
The detections within the delay are ignored.

The duration is specified as a number with a unit suffix ("ns", "us" (or "µs"), "ms", "s", "m", "h").

### -s, --signal signal

This signal will be sent to stop the command on restart.
The default signal is `SIGTERM`.

This option can be `SIGHUP`, `SIGINT`, `SIGKILL`, `SIGUSR1`, `SIGUSR2`, or `SIGTERM`.

This options has no effect on Windows.

#### -v, --verbose

Output logs verbosely.

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

## Referenced project

 - [realize](https://github.com/oxequa/realize)
 - [fresh](https://github.com/gravityblast/fresh)
 - [gin](https://github.com/codegangsta/gin)
 - [go-task](https://github.com/go-task/task)
