package dnsanitize

import (
	"github.com/nil0x42/dnsanity/internal/dns"
)

// ServerPool streams huge resolver lists in small batches to save memory.
// All methods are single-goroutine – no mutex needed.
type ServerPool struct {
	/* immutable ----------------------------------------------------------- */
	template    dns.Template               // checks
	maxAttempts int                        // attempts per check

	/* dynamic ------------------------------------------------------------- */
	todo        []string                   // IPs still to load
	nextTodo    int                        // cursor in todo
	pool        map[int]*dns.ServerContext // srvID ➜ *ServerContext
	nextSlot    int                        // srvID generator
	maxPoolSize int                        // maximum pool size
}

/* construction ----------------------------------------------------------- */

// NewServerPool clones todoList and instantly fills the pool.
func NewServerPool(
	todoList    []string,
	template    dns.Template,
	maxPoolSize int,
	maxAttempts int,
) *ServerPool {
	sp := &ServerPool{
		template:        template,
		maxAttempts:     maxAttempts,
		todo:            append([]string(nil), todoList...),
		pool:            make(map[int]*dns.ServerContext),
		maxPoolSize:     maxPoolSize,
	}
	return sp
}

/* public API ------------------------------------------------------------- */

// LoadN loads up to n ServerContexts into the pool, expanding it as needed.
// Returns the number of ServerContexts inserted.
func (sp *ServerPool) LoadN(n int) int {
    inserted := 0
    // Insert until we've done n inserts or run out of todos
    for inserted < n && !sp.IsFull() && sp.NumPending() > 0 {
        ip := sp.todo[sp.nextTodo]
        sp.nextTodo++
        slot := sp.nextSlot
        sp.pool[slot] = dns.NewServerContext(ip, sp.template, sp.maxAttempts)
        sp.nextSlot++
        inserted++
    }
    return inserted
}

// Unload removes a finished ServerContext; if not Disabled, records its IP.
func (sp *ServerPool) Unload(slot int) {
	delete(sp.pool, slot)
}

// Len returns current servers loaded in pool
func (sp *ServerPool) Len() int {
	return len(sp.pool)
}

// NumPending tells how many servers must still be sent to queue
func (sp *ServerPool) NumPending() int {
	return len(sp.todo) - sp.nextTodo
}

// IsDrained is true when no server remains and todo is empty.
func (sp *ServerPool) IsDrained() bool {
	return sp.Len() == 0 && sp.NumPending() == 0
}

// IsFull is true when pool size == maxPoolSize
func (sp *ServerPool) IsFull() bool {
	return sp.Len() == sp.maxPoolSize
}
