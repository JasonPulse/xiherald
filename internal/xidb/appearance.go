package xidb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Character appearance, which is what a portrait renderer needs.
//
// There are two sources and they are not interchangeable:
//
//   - char_look holds MODEL ids for each visible slot. This is the normal case.
//   - char_style holds ITEM ids, applied when chars.isstylelocked is set. The
//     server compares those values with HasItem (charutils.cpp:1961), which is
//     what gives them away as item ids rather than model ids.
//
// So a style-locked character needs char_style's item id mapped through
// item_equipment.MId to get a model id. Rendering char_style's raw values as
// models would silently dress everyone in the wrong gear.
//
// The slot order here matches Vellichor's EntityLook, so the render work-list
// maps straight onto its VELLICHOR_PC_EQUIP argument.

// AppearanceSlots are the visible equipment slots, in EntityLook order.
var AppearanceSlots = []string{"head", "body", "hands", "legs", "feet", "main", "sub", "ranged"}

// Appearance is one character's renderable look.
type Appearance struct {
	CharID      int
	Name        string
	Race        int
	Face        int
	Size        int
	StyleLocked bool

	// Models is indexed the same as AppearanceSlots.
	Models [8]int
}

func (a Appearance) RaceName() string { return RaceName(a.Race) }

// Renderable reports whether there is enough to draw. Race zero means the
// character has no char_look row at all, which no renderer can do anything with.
func (a Appearance) Renderable() bool { return a.Race > 0 }

// EquipArg renders the slot list the way Vellichor's VELLICHOR_PC_EQUIP wants
// it: "head=12,body=135,...". Empty slots are omitted rather than sent as zero,
// because zero is a real model id for some slots and "unequipped" is not.
func (a Appearance) EquipArg() string {
	parts := make([]string, 0, len(AppearanceSlots))
	for i, slot := range AppearanceSlots {
		if a.Models[i] > 0 {
			parts = append(parts, slot+"="+strconv.Itoa(a.Models[i]))
		}
	}
	return strings.Join(parts, ",")
}

// Hash identifies this appearance. A renderer keeps the hash it last drew and
// skips the character when it has not moved, which is the difference between
// re-rendering everyone on every run and re-rendering the two people who
// changed gear.
func (a Appearance) Hash() string {
	sum := sha256.New()
	fmt.Fprintf(sum, "1|%d|%d|%d|%t", a.Race, a.Face, a.Size, a.StyleLocked)
	for _, m := range a.Models {
		fmt.Fprintf(sum, "|%d", m)
	}
	return hex.EncodeToString(sum.Sum(nil))[:12]
}

// appearanceQuery resolves both sources in one read. The eight item_equipment
// joins are primary-key lookups against a small table.
func appearanceQuery(where string) string {
	var lookCols, styleJoins, styleCols strings.Builder

	for _, slot := range AppearanceSlots {
		fmt.Fprintf(&lookCols, ",\n\t  COALESCE(l.%s, 0)", slot)
		fmt.Fprintf(&styleCols, ",\n\t  COALESCE(eq_%s.MId, 0)", slot)
		fmt.Fprintf(&styleJoins,
			"\n\tLEFT JOIN item_equipment eq_%s ON eq_%s.itemId = st.%s", slot, slot, slot)
	}

	return `
	SELECT
	  c.charid,
	  c.charname,
	  c.isstylelocked,
	  COALESCE(l.race, 0),
	  COALESCE(l.face, 0),
	  COALESCE(l.size, 0)` + lookCols.String() + styleCols.String() + `
	FROM chars c
	LEFT JOIN char_look  l  ON l.charid  = c.charid
	LEFT JOIN char_style st ON st.charid = c.charid` + styleJoins.String() + `
	WHERE ` + where
}

func scanAppearance(scan func(...any) error) (Appearance, error) {
	var (
		a      Appearance
		locked int
		look   [8]int
		style  [8]int
	)

	targets := []any{&a.CharID, &a.Name, &locked, &a.Race, &a.Face, &a.Size}
	for i := range look {
		targets = append(targets, &look[i])
	}
	for i := range style {
		targets = append(targets, &style[i])
	}

	if err := scan(targets...); err != nil {
		return a, err
	}

	a.StyleLocked = locked == 1
	for i := range a.Models {
		// A style-locked slot with no mapped model falls back to the real one,
		// which is what the client does for an empty style slot.
		if a.StyleLocked && style[i] > 0 {
			a.Models[i] = style[i]
		} else {
			a.Models[i] = look[i]
		}
	}

	return a, nil
}

// Appearances is the render work-list: every real character, cheapest sort.
func (db *DB) Appearances(ctx context.Context) ([]Appearance, error) {
	return cached(db.cache, "appearances", func() ([]Appearance, error) {
		rows, err := db.sql.QueryContext(ctx,
			appearanceQuery(realCharacter+" ORDER BY c.charname"))
		if err != nil {
			return nil, fmt.Errorf("appearances: %w", err)
		}
		defer rows.Close()

		out := []Appearance{}
		for rows.Next() {
			a, err := scanAppearance(rows.Scan)
			if err != nil {
				return nil, fmt.Errorf("appearance scan: %w", err)
			}
			out = append(out, a)
		}
		return out, rows.Err()
	})
}

// AppearanceOf loads one character's look by name.
func (db *DB) AppearanceOf(ctx context.Context, name string) (Appearance, error) {
	if strings.TrimSpace(name) == "" {
		return Appearance{}, ErrNotFound
	}

	return cached(db.cache, "appearance:"+strings.ToLower(name), func() (Appearance, error) {
		row := db.sql.QueryRowContext(ctx,
			appearanceQuery("c.charname = ? AND "+realCharacter+" LIMIT 1"), name)

		a, err := scanAppearance(row.Scan)
		if isNoRows(err) {
			return Appearance{}, ErrNotFound
		}
		if err != nil {
			return Appearance{}, fmt.Errorf("appearance of %q: %w", name, err)
		}
		return a, nil
	})
}
