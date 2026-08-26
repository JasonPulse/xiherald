package xidb

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RosterRow is one character on the roster page.
type RosterRow struct {
	CharID      int
	Name        string
	Nation      int
	Race        int
	MainJob     int
	MainLevel   int
	SubJob      int
	SubLevel    int
	MasterLevel int
	TitleID     int
	Deaths      int
	Kills       int
	KOs         int
	TotalLevels int
	JobsCapped  int
	Playtime    int
	ZoneID      int
	ZoneName    string
	Created     time.Time
	LastLogout  time.Time
	Online      bool
}

func (r RosterRow) NationName() string { return NationName(r.Nation) }
func (r RosterRow) NationSlug() string { return NationSlug(r.Nation) }
func (r RosterRow) RaceName() string   { return RaceName(r.Race) }
func (r RosterRow) RaceSex() string    { return RaceSex(r.Race) }
func (r RosterRow) Title() string      { return TitleName(r.TitleID) }

// Zone is the character's location, readable.
func (r RosterRow) Zone() string { return ZoneDisplay(r.ZoneName) }

// Job renders the familiar main/sub pair, with the master level appended when
// the character has one. A character who has never picked a job has no pair to
// render.
func (r RosterRow) Job() string {
	if r.MainJob == 0 {
		return "No job"
	}

	main := fmt.Sprintf("%s%d", JobShort(r.MainJob), r.MainLevel)
	if r.MasterLevel > 0 {
		main = fmt.Sprintf("%s (ML%d)", main, r.MasterLevel)
	}
	if r.SubJob == 0 || r.SubLevel == 0 {
		return main
	}
	return fmt.Sprintf("%s / %s%d", main, JobShort(r.SubJob), r.SubLevel)
}

// PlaytimeHours is what the roster column shows. chars.playtime counts seconds.
func (r RosterRow) PlaytimeHours() float64 { return float64(r.Playtime) / 3600.0 }

// KD is kills per knockout. A character who has never been knocked out reports
// their kill count outright rather than dividing by zero.
func (r RosterRow) KD() float64 {
	if r.KOs == 0 {
		return float64(r.Kills)
	}
	return float64(r.Kills) / float64(r.KOs)
}

// rosterSorts whitelists the ?sort= values. Interpolating user input into an
// ORDER BY is only safe because the value is looked up here and never used.
var rosterSorts = map[string]string{
	"name":     "c.charname ASC",
	"level":    "main_level DESC, master_level DESC, c.charname ASC",
	"jobs":     "jobs_capped DESC, total_levels DESC, c.charname ASC",
	"levels":   "total_levels DESC, c.charname ASC",
	"playtime": "c.playtime DESC, c.charname ASC",
	"kills":    "kills DESC, c.charname ASC",
	"deaths":   "deaths DESC, c.charname ASC",
	"seen":     "online DESC, c.last_logout DESC",
	"created":  "c.timecreated ASC",
}

// RosterSortOrder is the order the column headers offer the sorts in.
var RosterSortOrder = []struct{ Slug, Label string }{
	{"seen", "Last seen"},
	{"name", "Name"},
	{"level", "Level"},
	{"jobs", "Jobs at 99"},
	{"levels", "Total levels"},
	{"playtime", "Playtime"},
	{"kills", "Enemies defeated"},
	{"deaths", "Deaths"},
	{"created", "Oldest"},
}

// jobSum builds the "sum of every job level" and "count of capped jobs" SQL
// expressions from JobColumns, so adding a job to the game means touching only
// that one list.
func jobSum() (total, capped string) {
	sums := make([]string, 0, len(JobColumns))
	caps := make([]string, 0, len(JobColumns))
	for _, col := range JobColumns {
		sums = append(sums, "COALESCE(j."+col+",0)")
		caps = append(caps, "IF(COALESCE(j."+col+",0) >= 99, 1, 0)")
	}
	return strings.Join(sums, " + "), strings.Join(caps, " + ")
}

const rosterColumns = `
	  c.charid,
	  c.charname,
	  c.nation,
	  c.playtime,
	  c.timecreated,
	  c.last_logout,
	  c.pos_zone,
	  COALESCE(z.name, '')                AS zone_name,
	  COALESCE(l.race, 0)                 AS race,
	  COALESCE(s.mjob, 0)                 AS main_job,
	  COALESCE(s.mlvl, 0)                 AS main_level,
	  COALESCE(s.sjob, 0)                 AS sub_job,
	  COALESCE(s.slvl, 0)                 AS sub_level,
	  COALESCE(s.master_level, 0)         AS master_level,
	  COALESCE(s.title, 0)                AS title_id,
	  COALESCE(s.death, 0)                AS deaths,
	  COALESCE(h.enemies_defeated, 0)     AS kills,
	  COALESCE(h.times_knocked_out, 0)    AS kos`

const rosterJoins = `
	FROM chars c
	LEFT JOIN char_stats        s    ON s.charid    = c.charid
	LEFT JOIN char_look         l    ON l.charid    = c.charid
	LEFT JOIN char_history      h    ON h.charid    = c.charid
	LEFT JOIN char_jobs         j    ON j.charid    = c.charid
	LEFT JOIN zone_settings     z    ON z.zoneid    = c.pos_zone
	LEFT JOIN accounts_sessions sess ON sess.charid = c.charid`

// Roster lists every character. sort must be a key of rosterSorts; anything
// else falls back to last-seen order.
func (db *DB) Roster(ctx context.Context, sort string) ([]RosterRow, error) {
	order, ok := rosterSorts[sort]
	if !ok {
		sort = "seen"
		order = rosterSorts[sort]
	}

	return cached(db.cache, "roster:"+sort, func() ([]RosterRow, error) {
		totalExpr, cappedExpr := jobSum()

		query := `SELECT` + rosterColumns + `,
	  ` + totalExpr + `  AS total_levels,
	  ` + cappedExpr + ` AS jobs_capped,
	  sess.charid IS NOT NULL AS online` +
			rosterJoins + `
	ORDER BY ` + order

		rows, err := db.sql.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("roster query: %w", err)
		}
		defer rows.Close()

		var out []RosterRow
		for rows.Next() {
			var r RosterRow
			if err := rows.Scan(
				&r.CharID, &r.Name, &r.Nation, &r.Playtime, &r.Created,
				&r.LastLogout, &r.ZoneID, &r.ZoneName, &r.Race, &r.MainJob,
				&r.MainLevel, &r.SubJob, &r.SubLevel, &r.MasterLevel,
				&r.TitleID, &r.Deaths, &r.Kills, &r.KOs,
				&r.TotalLevels, &r.JobsCapped, &r.Online,
			); err != nil {
				return nil, fmt.Errorf("roster scan: %w", err)
			}
			out = append(out, r)
		}
		return out, rows.Err()
	})
}

// ServerSummary is the strip of numbers above the roster.
type ServerSummary struct {
	Characters   int
	Online       int
	TotalKills   int
	TotalDeaths  int
	TotalHours   float64
	JobsAt99     int
	NewestChar   string
	NewestJoined time.Time
}

func (db *DB) Summary(ctx context.Context) (ServerSummary, error) {
	return cached(db.cache, "summary", func() (ServerSummary, error) {
		var s ServerSummary
		_, cappedExpr := jobSum()

		query := `
		SELECT
		  COUNT(*),
		  COALESCE(SUM(sess.charid IS NOT NULL), 0),
		  COALESCE(SUM(h.enemies_defeated), 0),
		  COALESCE(SUM(st.death), 0),
		  COALESCE(SUM(c.playtime), 0) / 3600.0,
		  COALESCE(SUM(` + cappedExpr + `), 0)
		FROM chars c
		LEFT JOIN char_history      h    ON h.charid    = c.charid
		LEFT JOIN char_stats        st   ON st.charid   = c.charid
		LEFT JOIN char_jobs         j    ON j.charid    = c.charid
		LEFT JOIN accounts_sessions sess ON sess.charid = c.charid`

		if err := db.sql.QueryRowContext(ctx, query).Scan(
			&s.Characters, &s.Online, &s.TotalKills, &s.TotalDeaths,
			&s.TotalHours, &s.JobsAt99,
		); err != nil {
			return s, fmt.Errorf("summary query: %w", err)
		}

		// Newest character is a separate read so an empty chars table still
		// yields a usable summary rather than a NULL scan error.
		err := db.sql.QueryRowContext(ctx,
			`SELECT charname, timecreated FROM chars ORDER BY timecreated DESC LIMIT 1`,
		).Scan(&s.NewestChar, &s.NewestJoined)
		if err != nil && !isNoRows(err) {
			return s, fmt.Errorf("summary newest: %w", err)
		}

		return s, nil
	})
}
