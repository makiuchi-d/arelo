package fspoll

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sync"
	"time"
)

// Poller is a polling watcher for file changes.
type Poller struct {
	events chan Event
	errors chan error

	interval time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	mu         sync.RWMutex
	closed     bool
	cancellers map[string]context.CancelFunc
}

// New generates a new Poller.
func New(interval time.Duration) *Poller {
	ctx, cancel := context.WithCancel(context.Background())
	return &Poller{
		events:     make(chan Event, 1),
		errors:     make(chan error, 1),
		interval:   interval,
		ctx:        ctx,
		cancel:     cancel,
		cancellers: make(map[string]context.CancelFunc),
	}
}

// Add starts watching the path for changes.
func (p *Poller) Add(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}

	fi, err := os.Stat(name)
	if err != nil {
		return err
	}

	if _, ok := p.cancellers[name]; ok {
		return nil // already watching
	}

	ctx, cancel := context.WithCancel(p.ctx)
	p.cancellers[name] = cancel

	go func() {
		if fi.IsDir() {
			p.pollingDir(ctx, name, fi)
		} else {
			p.pollingFile(ctx, name, fi)
		}
		_ = p.Remove(name)
	}()

	return nil
}

// Close stops all watches and closes the channels.
func (p *Poller) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	p.cancel()

	return nil
}

// Remove stops watching the specified path.
func (p *Poller) Remove(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	cancel, ok := p.cancellers[name]
	if !ok {
		return ErrNonExistentWatch
	}

	cancel()
	delete(p.cancellers, name)

	return nil
}

// WatchList returns a list of watching path names.
func (p *Poller) WatchList() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	names := make([]string, 0, len(p.cancellers))
	for name := range p.cancellers {
		names = append(names, name)
	}
	return names
}

// Events returns a channel that receives filesystem events.
func (p *Poller) Events() <-chan Event {
	return p.events
}

// Errors returns a channel that receives errors.
func (p *Poller) Errors() <-chan error {
	return p.errors
}

func (p *Poller) pollingDir(ctx context.Context, name string, fi fs.FileInfo) {
}

func (p *Poller) pollingFile(ctx context.Context, name string, fi fs.FileInfo) {
	t := time.NewTicker(p.interval)

	mode := fi.Mode()
	modt := fi.ModTime()
	size := fi.Size()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		fi, err := os.Stat(name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				p.sendEvent(ctx, name, Remove)
				return
			}
			if !p.sendError(ctx, err) {
				return
			}
		}

		if m := fi.Mode(); m != mode {
			mode = m
			if !p.sendEvent(ctx, name, Chmod) {
				return
			}
		}

		if m, s := fi.ModTime(), fi.Size(); m != modt || s != size {
			modt = m
			size = s
			if !p.sendEvent(ctx, name, Write) {
				return
			}
		}
	}
}

func (p *Poller) isClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

func (p *Poller) sendEvent(ctx context.Context, name string, op Op) bool {
	if p.isClosed() {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case p.events <- Event{Name: name, Op: op}:
		return true
	}
}

func (p *Poller) sendError(ctx context.Context, err error) bool {
	if p.isClosed() {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case p.errors <- err:
		return true
	}
}
