package xidb

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"
)

// Portrait storage.
//
// Portraits live in their own database, separate from the game database. The
// Herald has full rights there and SELECT only on the game tables, so the two
// postures stay distinct: nothing here can write to xidb, and the read-only
// guarantee over game data is unchanged.
//
// The Herald does not write portraits either. The renderer connects to this
// database itself, which keeps the Herald free of any network write path. There
// is no upload endpoint and no shared secret to leak if the site is ever put
// behind a tunnel.

// safeSchema guards the one identifier that gets interpolated rather than
// bound. Schema names cannot be query parameters, so the value is constrained
// instead of trusted.
var safeSchema = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// Portrait is one rendered character image.
type Portrait struct {
	CharID      int
	Hash        string
	ContentType string
	Bytes       []byte
	Width       int
	Height      int
	RenderedAt  time.Time
}

// PortraitStore reads portraits. Writes are the renderer's job.
type PortraitStore struct {
	db     *sql.DB
	schema string
	ready  bool
}

// PortraitSchemaDDL is the table definition, also emitted by
// tools/portraits/schema.sql so the renderer and the Herald cannot disagree
// about the shape.
func PortraitSchemaDDL(schema string) string {
	return `CREATE TABLE IF NOT EXISTS ` + schema + `.portraits (
	  charid       int(10) unsigned NOT NULL,
	  hash         varchar(32)      NOT NULL,
	  content_type varchar(32)      NOT NULL DEFAULT 'image/png',
	  width        smallint(5) unsigned NOT NULL DEFAULT 0,
	  height       smallint(5) unsigned NOT NULL DEFAULT 0,
	  bytes        mediumblob       NOT NULL,
	  rendered_at  timestamp        NOT NULL DEFAULT CURRENT_TIMESTAMP
	                                ON UPDATE CURRENT_TIMESTAMP,
	  PRIMARY KEY (charid)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci`
}

// OpenPortraits prepares the store. A schema that is missing or unreachable is
// not an error: the Herald comes up with portraits disabled and every page
// still renders. Portraits are a decoration, not a dependency.
func (db *DB) OpenPortraits(ctx context.Context, schema string) (*PortraitStore, error) {
	if schema == "" {
		return &PortraitStore{}, nil
	}
	if !safeSchema.MatchString(schema) {
		return &PortraitStore{}, fmt.Errorf("portrait schema %q is not a bare identifier", schema)
	}

	store := &PortraitStore{db: db.sql, schema: schema}

	if _, err := db.sql.ExecContext(ctx, PortraitSchemaDDL(schema)); err != nil {
		return store, fmt.Errorf("ensure %s.portraits: %w", schema, err)
	}

	store.ready = true
	return store, nil
}

// Enabled reports whether portraits can be served.
func (p *PortraitStore) Enabled() bool { return p != nil && p.ready }

// Get returns one portrait. A missing row is (nil, nil) so a caller can answer
// 404 without treating it as a failure.
func (p *PortraitStore) Get(ctx context.Context, charID int) (*Portrait, error) {
	if !p.Enabled() {
		return nil, nil
	}

	var out Portrait
	err := p.db.QueryRowContext(ctx,
		`SELECT charid, hash, content_type, width, height, bytes, rendered_at
		 FROM `+p.schema+`.portraits WHERE charid = ?`, charID,
	).Scan(&out.CharID, &out.Hash, &out.ContentType, &out.Width, &out.Height,
		&out.Bytes, &out.RenderedAt)

	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get portrait %d: %w", charID, err)
	}
	return &out, nil
}

// HashFor returns the hash of the stored portrait for one character, and false
// when none has been rendered.
//
// The URL must carry the STORED hash, not the current appearance hash. Using
// the appearance hash would mint a fresh URL the moment somebody changes gear,
// before anything has re-rendered, and the browser would then cache the stale
// image under that new URL forever because the response is immutable.
func (p *PortraitStore) HashFor(ctx context.Context, charID int) (string, bool) {
	if !p.Enabled() {
		return "", false
	}

	var hash string
	err := p.db.QueryRowContext(ctx,
		`SELECT hash FROM `+p.schema+`.portraits WHERE charid = ?`, charID).Scan(&hash)
	if err != nil {
		return "", false
	}
	return hash, hash != ""
}

// Have returns the charids that already have a portrait, mapped to the
// appearance hash each was rendered from. The character page uses it to decide
// whether to emit an img tag at all, so an unrendered character produces no
// request rather than a 404 per page view.
func (p *PortraitStore) Have(ctx context.Context) (map[int]string, error) {
	out := map[int]string{}
	if !p.Enabled() {
		return out, nil
	}

	rows, err := p.db.QueryContext(ctx, `SELECT charid, hash FROM `+p.schema+`.portraits`)
	if err != nil {
		return out, fmt.Errorf("list portraits: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return out, fmt.Errorf("scan portrait row: %w", err)
		}
		out[id] = hash
	}
	return out, rows.Err()
}
