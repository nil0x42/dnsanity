package dnsanitize

import (
	"github.com/nil0x42/dnsanity/internal/dns"
)

// ServerPool streams huge resolver lists in small batches to save memory.
// All methods are single-goroutine – no mutex needed.
type ServerPool struct {
	template    dns.Template               // checks

	queue       []string                   // IPs still to load
	dequeueIdx  int                        // next server idx to dequeue

	pool        map[int]*dns.ServerContext // srvID ➜ *ServerContext
	nextSlot    int                        // srvID generator
	idealPoolSz int                        // ideal pool size
	maxPoolSz   int                        // maximum pool size

	maxAttempts int                        // needed to build ServerContext
}

/* construction ----------------------------------------------------------- */

// NewServerPool
func NewServerPool(
	serverIPs   []string,
	template    dns.Template,
	idealPoolSz int,
	maxPoolSz   int,
	maxAttempts int,
) *ServerPool {
	if (idealPoolSz > maxPoolSz) {
		panic("idealPoolSz > maxPoolSz")
	}
	sp := &ServerPool{
		template:        template,
		queue:           serverIPs,
		pool:            make(map[int]*dns.ServerContext, maxPoolSz),
		idealPoolSz:     idealPoolSz,
		maxPoolSz:       maxPoolSz,
		maxAttempts:     maxAttempts,
	}
	// sp.LoadN(idealPoolSz)
	return sp
}

/* public API ------------------------------------------------------------- */

// LoadN loads up to n ServerContexts into the pool, expanding it as needed.
// Returns the number of ServerContexts inserted.
func (sp *ServerPool) LoadN(n int) int {
	inserted := 0
	for inserted < n && !sp.IsFull() && sp.NumPending() > 0 {
		ip := sp.queue[sp.dequeueIdx]
		sp.dequeueIdx++
		sp.pool[sp.nextSlot] = dns.NewServerContext(
			ip, sp.template, sp.maxAttempts,
		)
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
	return len(sp.queue) - sp.dequeueIdx
}

// IsDrained is true when no server remains and queue is empty.
func (sp *ServerPool) IsDrained() bool {
	return sp.Len() == 0 && sp.NumPending() == 0
}

// IsFull is true when pool size == maxPoolSz
func (sp *ServerPool) IsFull() bool {
	return sp.Len() == sp.maxPoolSz
}

// IsOverLoaded is true when pool size >= idealPoolSz
func (sp *ServerPool) IsOverLoaded() bool {
	return sp.Len() >= sp.idealPoolSz
}
