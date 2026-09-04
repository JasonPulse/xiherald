package xidb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Equipped gear, read from char_equip_saved.
//
// There are two tables that could answer this and they are not equivalent.
// char_equip holds what is equipped right now, but only as an indirection into
// char_inventory, so reading it would require granting the Herald itemId on
// that table, and with it the ability to read every item every character owns.
// char_equip_saved holds item ids directly, keyed by (charid, jobid), so it
// needs no inventory access at all.
//
// The trade is staleness for blast radius: this is the set LSB last saved for
// that job rather than the live one. Given the Herald is a public-facing
// read-only site, keeping inventories unreadable is worth more than being
// current to the minute.

// EquipSlots are the sixteen equipment slots in xi.slot order, paired with
// their column name in char_equip_saved.
var EquipSlots = []struct {
	Column string
	Label  string
}{
	{"main", "Main"},
	{"sub", "Sub"},
	{"ranged", "Ranged"},
	{"ammo", "Ammo"},
	{"head", "Head"},
	{"body", "Body"},
	{"hands", "Hands"},
	{"legs", "Legs"},
	{"feet", "Feet"},
	{"neck", "Neck"},
	{"waist", "Waist"},
	{"ear1", "Earring"},
	{"ear2", "Earring"},
	{"ring1", "Ring"},
	{"ring2", "Ring"},
	{"back", "Back"},
}

// EquipPiece is one filled slot.
type EquipPiece struct {
	Slot   string
	Label  string
	ItemID int
	Name   string
}

// Loadout is a character's saved gear for one job.
type Loadout struct {
	JobID   int
	Job     string
	JobName string
	Pieces  []EquipPiece
	// OtherJobs are the other jobs this character has a saved set for, which
	// is the only hint the page can give that more gear exists.
	OtherJobs []string
}

func (l Loadout) Empty() bool { return len(l.Pieces) == 0 }

// smallWords stay lowercase inside an item name.
var smallWords = map[string]bool{
	"of": true, "the": true, "and": true, "in": true, "a": true, "de": true,
}

// ItemDisplayName turns item_basic.name into something readable.
//
// The database stores names lowercased with underscores and with punctuation
// stripped: kirins_osode is really "Kirin's Osode". The apostrophe cannot be
// recovered here the way it can for zone names, because those kept a capital
// letter at the seam (dOria) and these do not. A possessive heuristic on a
// trailing 's' would render brass_cap as "Bras's Cap", so nothing is guessed:
// the name comes out as "Kirins Osode", which is wrong in a way a reader can
// see through rather than wrong in a way that looks authoritative.
func ItemDisplayName(raw string) string {
	if raw == "" {
		return ""
	}

	words := strings.Split(raw, "_")
	out := make([]string, 0, len(words))

	for i, w := range words {
		if w == "" {
			continue
		}
		// Upgrade markers and numerals are already correct as written.
		if w[0] == '+' || isRomanNumeral(w) {
			out = append(out, strings.ToUpper(w))
			continue
		}
		if i > 0 && smallWords[w] {
			out = append(out, w)
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		out = append(out, string(r))
	}

	return strings.Join(out, " ")
}

func isRomanNumeral(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		switch unicode.ToLower(r) {
		case 'i', 'v', 'x':
		default:
			return false
		}
	}
	return true
}

// Loadout reads the saved gear for one character and job.
func (db *DB) Loadout(ctx context.Context, charID, jobID int) (Loadout, error) {
	key := "loadout:" + strconv.Itoa(charID) + ":" + strconv.Itoa(jobID)

	return cached(db.cache, key, func() (Loadout, error) {
		out := Loadout{JobID: jobID, Job: JobShort(jobID), JobName: JobFull(jobID)}

		cols := make([]string, 0, len(EquipSlots))
		for _, s := range EquipSlots {
			cols = append(cols, s.Column)
		}

		ids := make([]int, len(EquipSlots))
		targets := make([]any, len(EquipSlots))
		for i := range ids {
			targets[i] = &ids[i]
		}

		err := db.sql.QueryRowContext(ctx,
			`SELECT `+strings.Join(cols, ", ")+`
			 FROM char_equip_saved WHERE charid = ? AND jobid = ?`,
			charID, jobID).Scan(targets...)
		if isNoRows(err) {
			// No saved set for this job is normal, not an error.
			return db.withOtherJobs(ctx, charID, jobID, out)
		}
		if err != nil {
			return out, fmt.Errorf("loadout %d/%d: %w", charID, jobID, err)
		}

		names, err := db.itemNames(ctx, ids)
		if err != nil {
			return out, err
		}

		for i, s := range EquipSlots {
			if ids[i] == 0 {
				continue
			}
			name := names[ids[i]]
			if name == "" {
				// An id with no item_basic row still tells the reader a slot is
				// filled, which beats hiding it.
				name = fmt.Sprintf("Item #%d", ids[i])
			}
			out.Pieces = append(out.Pieces, EquipPiece{
				Slot: s.Column, Label: s.Label, ItemID: ids[i], Name: name,
			})
		}

		return db.withOtherJobs(ctx, charID, jobID, out)
	})
}

// itemNames resolves ids to display names in one round trip.
func (db *DB) itemNames(ctx context.Context, ids []int) (map[int]string, error) {
	wanted := make([]any, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			wanted = append(wanted, id)
		}
	}

	out := map[int]string{}
	if len(wanted) == 0 {
		return out, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(wanted)), ",")
	rows, err := db.sql.QueryContext(ctx,
		`SELECT itemid, name FROM item_basic WHERE itemid IN (`+placeholders+`)`,
		wanted...)
	if err != nil {
		return out, fmt.Errorf("item names: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return out, fmt.Errorf("item name scan: %w", err)
		}
		out[id] = ItemDisplayName(raw)
	}

	return out, rows.Err()
}

// withOtherJobs records which other jobs have a saved set.
func (db *DB) withOtherJobs(ctx context.Context, charID, exceptJob int, out Loadout) (Loadout, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT jobid FROM char_equip_saved WHERE charid = ? AND jobid <> ? ORDER BY jobid`,
		charID, exceptJob)
	if err != nil {
		return out, fmt.Errorf("other loadouts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return out, fmt.Errorf("other loadout scan: %w", err)
		}
		out.OtherJobs = append(out.OtherJobs, JobShort(id))
	}

	return out, rows.Err()
}
