package dnsanitize

import (
	"time"
	"context"

	"github.com/nil0x42/dnsanity/internal/dns"
)

// ServerPool streams huge resolver lists in small batches to save memory.
// All methods are single-goroutine – no mutex needed.
type ServerPool struct {
	/* immutable ----------------------------------------------------------- */
	template    dns.Template           // checks
	reqInterval time.Duration          // min delay per server
	maxAttempts int                    // attempts per check

	/* dynamic ------------------------------------------------------------- */
	todo        []string               // IPs still to load
	nextTodo    int                    // cursor in todo
	pool        map[int]*dns.ServerContext // slotID ➜ *ServerContext
	nextSlot    int                    // slotID generator
	validIPs    []string               // successfully validated
	defaultPoolSize int // initial pool size
	maxPoolSize int // maximum pool size
}

/* construction ----------------------------------------------------------- */

// NewServerPool clones todoList and instantly fills the pool.
func NewServerPool(
	todoList    []string,
	template    dns.Template,
	poolSize    int,
	maxPoolSize int,
	reqInterval time.Duration,
	maxAttempts int,
) *ServerPool {
	if poolSize < 1 {
		panic("invalid poolSize <1")
	}
	if maxPoolSize < poolSize {
		panic("maxPoolsize < poolSize")
	}
	sp := &ServerPool{
		template:    template,
		reqInterval: reqInterval,
		maxAttempts: maxAttempts,
		todo:        append([]string(nil), todoList...),
		pool:        make(map[int]*dns.ServerContext, poolSize),
		defaultPoolSize: poolSize,
		maxPoolSize:     maxPoolSize,
	}
	sp.LoadN(poolSize)
	return sp
}

/* public API ------------------------------------------------------------- */

// LoadN loads up to n ServerContexts into the pool, expanding it as needed.
// Returns the number of ServerContexts inserted.
func (sp *ServerPool) LoadN(n int) int {
    inserted := 0
    // Insert until we've done n inserts or run out of todos
    for inserted < n && !sp.IsFull() && sp.nextTodo < len(sp.todo) {
        ip := sp.todo[sp.nextTodo]
        sp.nextTodo++
        slot := sp.nextSlot
        sp.pool[slot] = sp.newServerContext(ip)
        sp.nextSlot++
        inserted++
    }
    return inserted
}

// Unload removes a finished ServerContext; if not Disabled, records its IP.
func (sp *ServerPool) Unload(slot int) {
	if srv, ok := sp.pool[slot]; ok {
		if !srv.Disabled {
			sp.validIPs = append(sp.validIPs, srv.IPAddress)
		}
		delete(sp.pool, slot)
	}
}

// NumPending tells how many servers must still be sent to queue
func (sp *ServerPool) NumPending() int {
	return len(sp.todo) - sp.nextTodo
}

// IsDrained is true when no server remains and todo is empty.
func (sp *ServerPool) IsDrained() bool {
	return len(sp.pool) == 0 && sp.nextTodo >= len(sp.todo)
}

// IsFull is true when pool size == maxPoolSize
func (sp *ServerPool) IsFull() bool {
	return len(sp.pool) == sp.maxPoolSize
}

/* helpers ---------------------------------------------------------------- */

func (sp *ServerPool) newServerContext(ip string) *dns.ServerContext {
	ctx, cancelCtx := context.WithCancel(context.Background())
	sc := &dns.ServerContext{
		Ctx:			ctx,
		CancelCtx:		cancelCtx,
		Disabled:       false,
		IPAddress:      ip,
		FailedCount:    0,
		CompletedCount: 0,
		NextQueryAt:    time.Time{},
		PendingChecks:  make([]int, len(sp.template)),
		Checks:         make([]dns.CheckContext, len(sp.template)),
	}
	for i := range sp.template {
		sc.PendingChecks[i] = i
		sc.Checks[i].Answer.Domain = sp.template[i].Domain
		sc.Checks[i].Answer.Status = "SKIPPED"
		sc.Checks[i].AttemptsLeft = sp.maxAttempts
		sc.Checks[i].MaxAttempts = sp.maxAttempts
	}
	return sc
}
