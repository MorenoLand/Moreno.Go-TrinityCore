package scripting

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// reentrantMutex serializes Lua-state access like sync.Mutex, except a
// goroutine that already holds it may re-enter: hook handlers run under
// this lock (triggerNUpdated holds it across ProtectedCall), and a handler
// may call a Lua method that fires another hook (creature:Kill/Despawn,
// guild AddMember, group AddMember) — with a plain mutex that nested fire
// deadlocks on the same goroutine. The owner is the goroutine id parsed
// from runtime.Stack; a parse failure degrades to plain-mutex behavior.
// TryLockFree succeeds only when no goroutine holds the lock, so the
// guarded packet-fire probes keep their skip-if-held semantics even for
// the holding goroutine (a send-hook handler broadcasting must skip the
// nested send fire, not recurse into it).
type reentrantMutex struct {
	mu    sync.Mutex
	owner atomic.Int64
	depth atomic.Int32
}

func goroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	i := 0
	for i < n && (buf[i] < '0' || buf[i] > '9') {
		i++
	}
	var id int64
	for i < n && buf[i] >= '0' && buf[i] <= '9' {
		id = id*10 + int64(buf[i]-'0')
		i++
	}
	return id
}

func (m *reentrantMutex) Lock() {
	gid := goroutineID()
	if gid > 0 && m.owner.Load() == gid {
		m.depth.Add(1)
		return
	}
	m.mu.Lock()
	m.owner.Store(gid)
	m.depth.Store(1)
}

func (m *reentrantMutex) Unlock() {
	if m.depth.Add(-1) > 0 {
		return
	}
	m.owner.Store(0)
	m.mu.Unlock()
}

func (m *reentrantMutex) TryLockFree() bool {
	if m.depth.Load() != 0 {
		return false
	}
	if !m.mu.TryLock() {
		return false
	}
	m.owner.Store(goroutineID())
	m.depth.Store(1)
	return true
}
