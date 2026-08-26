// Package xidb reads the LandSandBoat game database.
//
// Everything here is read-only by construction: no statement in this package
// writes, and the deployment grants the Herald's MySQL user SELECT and nothing
// else. Queries are also capped by a short lock wait in the DSN, because the
// Herald sharing a database with a live game server means a slow leaderboard
// must never be the reason a player's zone-in stalls.
package xidb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	sql   *sql.DB
	cache *cache
}

func Open(dsn string, cacheTTL time.Duration) (*DB, error) {
	handle, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	// The Herald serves a handful of players. A small pool keeps its footprint
	// on the shared database negligible.
	handle.SetMaxOpenConns(4)
	handle.SetMaxIdleConns(2)
	handle.SetConnMaxLifetime(5 * time.Minute)

	return &DB{sql: handle, cache: newCache(cacheTTL)}, nil
}

func (db *DB) Close() error { return db.sql.Close() }

func (db *DB) Ping(ctx context.Context) error { return db.sql.PingContext(ctx) }

// cache holds rendered query results for a short window. Its job is to absorb
// page refreshes so repeated views cost the game database nothing.
type cache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]entry
}

type entry struct {
	value   any
	expires time.Time
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl, entries: map[string]entry{}}
}

// cached runs load unless a fresh result for key is already held. A load error
// is never cached, so a transient database failure does not stick.
func cached[T any](c *cache, key string, load func() (T, error)) (T, error) {
	var zero T
	if c == nil || c.ttl <= 0 {
		return load()
	}

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		if v, ok := e.value.(T); ok {
			return v, nil
		}
		return zero, fmt.Errorf("cache type mismatch for %q", key)
	}
	c.mu.Unlock()

	value, err := load()
	if err != nil {
		return zero, err
	}

	c.mu.Lock()
	c.entries[key] = entry{value: value, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()

	return value, nil
}

// isNoRows reports whether err is the driver's empty-result sentinel. Several
// reads here are optional (a character with no history row, an empty chars
// table) and should degrade to zero values rather than fail the page.
func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
