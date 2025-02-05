// fspoll provides polling file change watcher.
package fspoll

import (
	"github.com/fsnotify/fsnotify"
)

type Event = fsnotify.Event
type Op = fsnotify.Op

const (
	Create = fsnotify.Create
	Write  = fsnotify.Write
	Remove = fsnotify.Remove
	Rename = fsnotify.Rename
	Chmod  = fsnotify.Chmod
)

var (
	ErrNonExistentWatch = fsnotify.ErrNonExistentWatch
	ErrEventOverflow    = fsnotify.ErrEventOverflow
	ErrClosed           = fsnotify.ErrClosed
)

// Watcher is a common interface for fspoll and fsnotify
type Watcher interface {

	// Add starts watching the path for changes.
	Add(name string) error

	// Close stops all watches and closes the channels.
	Close() error

	// Remove stops watching the specified path.
	Remove(name string) error

	// WatchList returns a list of watching path names.
	WatchList() []string

	// Events returns a channel that receives filesystem events.
	Events() <-chan Event

	// Errors returns a channel that receives errors.
	Errors() <-chan error
}
