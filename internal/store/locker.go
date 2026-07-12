package store

import (
	"context"
	"sync"

	adksession "github.com/soasurs/adk/session"
)

type runLocker struct {
	mu      sync.Mutex
	entries map[adksession.RunLockKey]*runLock
}

type runLock struct {
	token chan struct{}
	refs  int
}

func newRunLocker() *runLocker {
	return &runLocker{entries: make(map[adksession.RunLockKey]*runLock)}
}

func (l *runLocker) LockRun(ctx context.Context, key adksession.RunLockKey) (func(), error) {
	l.mu.Lock()
	entry := l.entries[key]
	if entry == nil {
		entry = &runLock{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	select {
	case <-ctx.Done():
		l.releaseRef(key, entry)
		return nil, ctx.Err()
	case <-entry.token:
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			entry.token <- struct{}{}
			l.releaseRef(key, entry)
		})
	}, nil
}

func (l *runLocker) releaseRef(key adksession.RunLockKey, entry *runLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && len(entry.token) == 1 && l.entries[key] == entry {
		delete(l.entries, key)
	}
}
