package fspoll

import (
	"github.com/fsnotify/fsnotify"
)

// Wrapper of a fsnotify.Watcher.
type Wrapper struct {
	*fsnotify.Watcher
}

var _ Watcher = Wrapper{}

// Wrap returns a wrapping fsnotify.Watcher.
func Wrap(w *fsnotify.Watcher, err error) (Wrapper, error) {
	return Wrapper{w}, err
}

// Events returns Events channel of wrapping fsnotify.Watcher.
func (w Wrapper) Events() <-chan Event {
	return w.Watcher.Events
}

// Errors returns Errors channel of wrapping fsnotify.Watcher.
func (w Wrapper) Errors() <-chan error {
	return w.Watcher.Errors
}
