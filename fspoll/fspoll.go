// fspoll provides polling file change watcher.
package fspoll

import (
	"github.com/fsnotify/fsnotify"
)

type Event = fsnotify.Event

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

// Poller is a polling watcher for file changes.
type Poller struct {
	events chan Event
	errors chan error
}

var _ Watcher = &Poller{}

// New generates a new Poller.
func New() *Poller {
	return &Poller{
		events: make(chan Event),
		errors: make(chan error),
	}
}

// Add starts watching the path for changes.
func (p *Poller) Add(name string) error {
	return nil
}

// Close stops all watches and closes the channels.
func (p *Poller) Close() error {
	return nil
}

// Remove stops watching the specified path.
func (p *Poller) Remove(name string) error {
	return nil
}

// WatchList returns a list of watching path names.
func (p *Poller) WatchList() []string {
	return nil
}

// Events returns a channel that receives filesystem events.
func (p *Poller) Events() <-chan Event {
	return p.events
}

// Errors returns a channel that receives errors.
func (p *Poller) Errors() <-chan error {
	return p.errors
}
