package xidb

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotFound is returned when a character name has no row in chars.
var ErrNotFound = errors.New("character not found")

// HistoryColumns is the char_history table, labelled. The game server keeps
// every one of these counters per character with no extra work on our side,
// which is what makes the fun-stats page cheap.
var HistoryColumns = []struct {
	Column string
	Label  string
	Unit   string
}{
	{"enemies_defeated", "Enemies defeated", ""},
	{"times_knocked_out", "Times knocked out", ""},
	{"battles_fought", "Battles fought", ""},
	{"spells_cast", "Spells cast", ""},
	{"abilities_used", "Abilities used", ""},
	{"ws_used", "Weapon skills used", ""},
	{"items_used", "Items used", ""},
	{"distance_travelled", "Distance travelled", "yalms"},
	{"npc_interactions", "NPCs talked to", ""},
	{"chats_sent", "Chat lines sent", ""},
	{"joined_parties", "Parties joined", ""},
	{"joined_alliances", "Alliances joined", ""},
	{"mh_entrances", "Mog House visits", ""},
	{"gm_calls", "GM calls", ""},
}

// Format decides how a leaderboard value is printed.
type Format string

const (
	FormatInt   Format = "int"
	FormatRatio Format = "ratio"
	FormatHours Format = "hours"
)

// Metric is one leaderboard. Adding a stat to the Herald means adding one
// entry here and nothing else: the query, the page, the nav and the stats
// index all read from this registry.
type Metric struct {
	Slug   string
	Label  string
	Blurb  string
	Expr   string
	Unit   string
	Format Format
	Group  string
}

const (
	groupCombat      = "Combat"
	groupActivity    = "Activity"
	groupProgression = "Progression"
)

// Metrics is the leaderboard registry, in the order the stats index lists it.
var Metrics = buildMetrics()

func buildMetrics() []Metric {
	m := []Metric{
		{
			Slug:   "kills",
			Label:  "Enemies defeated",
			Blurb:  "Every mob that has died to this character.",
			Expr:   "COALESCE(h.enemies_defeated, 0)",
			Format: FormatInt,
			Group:  groupCombat,
		},
		{
			Slug:  "deaths",
			Label: "Deaths",
			Blurb: "Trips to the Home Point, the hard way. Counted from times " +
				"knocked out; char_stats.death is a weakness timer, not a tally.",
			Expr:   "COALESCE(h.times_knocked_out, 0)",
			Format: FormatInt,
			Group:  groupCombat,
		},
		{
			Slug:  "kd",
			Label: "Kill / death ratio",
			Blurb: "Enemies defeated per knockout. Never knocked out means the raw kill count.",
			Expr: "COALESCE(h.enemies_defeated, 0) / " +
				"GREATEST(COALESCE(h.times_knocked_out, 0), 1)",
			Format: FormatRatio,
			Group:  groupCombat,
		},
		{
			Slug:   "battles",
			Label:  "Battles fought",
			Blurb:  "Distinct engagements, not individual kills.",
			Expr:   "COALESCE(h.battles_fought, 0)",
			Format: FormatInt,
			Group:  groupCombat,
		},
		{
			Slug:   "weaponskills",
			Label:  "Weapon skills used",
			Blurb:  "TP well spent.",
			Expr:   "COALESCE(h.ws_used, 0)",
			Format: FormatInt,
			Group:  groupCombat,
		},
		{
			Slug:   "spells",
			Label:  "Spells cast",
			Blurb:  "Every successful cast, from Cure to Meteor.",
			Expr:   "COALESCE(h.spells_cast, 0)",
			Format: FormatInt,
			Group:  groupCombat,
		},
		{
			Slug:   "abilities",
			Label:  "Abilities used",
			Blurb:  "Job abilities, pet commands and the rest.",
			Expr:   "COALESCE(h.abilities_used, 0)",
			Format: FormatInt,
			Group:  groupCombat,
		},
		{
			Slug:   "playtime",
			Label:  "Playtime",
			Blurb:  "Hours logged in, straight off chars.playtime.",
			Expr:   "c.playtime / 3600.0",
			Unit:   "hours",
			Format: FormatHours,
			Group:  groupActivity,
		},
		{
			Slug:   "distance",
			Label:  "Distance travelled",
			Blurb:  "Yalms walked, run, ridden and chocoboed.",
			Expr:   "COALESCE(h.distance_travelled, 0)",
			Unit:   "yalms",
			Format: FormatInt,
			Group:  groupActivity,
		},
		{
			Slug:   "npcs",
			Label:  "NPCs talked to",
			Blurb:  "Conversations started. Quest hunters run high.",
			Expr:   "COALESCE(h.npc_interactions, 0)",
			Format: FormatInt,
			Group:  groupActivity,
		},
		{
			Slug:   "chat",
			Label:  "Chat lines sent",
			Blurb:  "Who actually talks.",
			Expr:   "COALESCE(h.chats_sent, 0)",
			Format: FormatInt,
			Group:  groupActivity,
		},
		{
			Slug:   "items",
			Label:  "Items used",
			Blurb:  "Potions, scrolls, food and fireworks.",
			Expr:   "COALESCE(h.items_used, 0)",
			Format: FormatInt,
			Group:  groupActivity,
		},
		{
			Slug:   "parties",
			Label:  "Parties joined",
			Blurb:  "How sociable this character has been.",
			Expr:   "COALESCE(h.joined_parties, 0)",
			Format: FormatInt,
			Group:  groupActivity,
		},
		{
			Slug:   "moghouse",
			Label:  "Mog House visits",
			Blurb:  "Time spent rearranging furniture.",
			Expr:   "COALESCE(h.mh_entrances, 0)",
			Format: FormatInt,
			Group:  groupActivity,
		},
		{
			Slug:   "levels",
			Label:  "Total job levels",
			Blurb:  "Every job level added together. The all-round grind score.",
			Expr:   "", // filled from JobColumns below
			Format: FormatInt,
			Group:  groupProgression,
		},
		{
			Slug:   "capped",
			Label:  "Jobs at 99",
			Blurb:  "Jobs taken to the level cap.",
			Expr:   "", // filled from JobColumns below
			Format: FormatInt,
			Group:  groupProgression,
		},
		{
			Slug:   "masterlevel",
			Label:  "Master level",
			Blurb:  "Post-99 progression, once Job Points are capped.",
			Expr:   "COALESCE(s.master_level, 0)",
			Format: FormatInt,
			Group:  groupProgression,
		},
		{
			Slug:   "merits",
			Label:  "Merit points",
			Blurb:  "Unspent merits currently held.",
			Expr:   "COALESCE(e.merits, 0)",
			Format: FormatInt,
			Group:  groupProgression,
		},
		{
			Slug:   "rankpoints",
			Label:  "Rank points",
			Blurb:  "Conquest standing with the home nation.",
			Expr:   "COALESCE(pr.rank_points, 0)",
			Format: FormatInt,
			Group:  groupProgression,
		},
		{
			Slug:   "jobpoints",
			Label:  "Job points",
			Blurb:  "Job points earned across every job.",
			Expr:   "COALESCE(jpt.total, 0)",
			Format: FormatInt,
			Group:  groupProgression,
		},
	}

	totalExpr, cappedExpr := jobSum()
	for i := range m {
		switch m[i].Slug {
		case "levels":
			m[i].Expr = totalExpr
		case "capped":
			m[i].Expr = cappedExpr
		}
	}
	return m
}

var metricBySlug = func() map[string]Metric {
	out := make(map[string]Metric, len(Metrics))
	for _, m := range Metrics {
		out[m.Slug] = m
	}
	return out
}()

func LookupMetric(slug string) (Metric, bool) {
	m, ok := metricBySlug[slug]
	return m, ok
}

// MetricGroups returns the registry bucketed for the stats index, preserving
// registry order within each group.
func MetricGroups() []struct {
	Name    string
	Metrics []Metric
} {
	order := []string{groupCombat, groupActivity, groupProgression}
	byName := map[string][]Metric{}
	for _, m := range Metrics {
		byName[m.Group] = append(byName[m.Group], m)
	}

	out := make([]struct {
		Name    string
		Metrics []Metric
	}, 0, len(order))
	for _, name := range order {
		out = append(out, struct {
			Name    string
			Metrics []Metric
		}{Name: name, Metrics: byName[name]})
	}
	return out
}

// LeaderRow is one line of a leaderboard.
type LeaderRow struct {
	Rank    int
	CharID  int
	Name    string
	Nation  int
	Race    int
	MainJob int
	MainLvl int
	Online  bool
	Value   float64
}

func (l LeaderRow) NationName() string { return NationName(l.Nation) }
func (l LeaderRow) NationSlug() string { return NationSlug(l.Nation) }
func (l LeaderRow) RaceName() string   { return RaceName(l.Race) }

func (l LeaderRow) JobText() string {
	if l.MainJob == 0 {
		return "---"
	}
	return fmt.Sprintf("%s%d", JobShort(l.MainJob), l.MainLvl)
}

// Leaderboard runs one metric. Rows whose value is zero are dropped, because a
// board of ties at nothing tells nobody anything.
func (db *DB) Leaderboard(ctx context.Context, m Metric) ([]LeaderRow, error) {
	return cached(db.cache, "leaders:"+m.Slug, func() ([]LeaderRow, error) {
		query := `
		SELECT charid, charname, nation, race, main_job, main_level, online, value
		FROM (
		    SELECT c.charid, c.charname, c.nation,
		           COALESCE(l.race, 0)          AS race,
		           COALESCE(s.mjob, 0)          AS main_job,
		           COALESCE(s.mlvl, 0)          AS main_level,
		           sess.charid IS NOT NULL      AS online,
		           ` + m.Expr + ` AS value
		    FROM chars c
		    LEFT JOIN char_stats        s    ON s.charid    = c.charid
		    LEFT JOIN char_look         l    ON l.charid    = c.charid
		    LEFT JOIN char_history      h    ON h.charid    = c.charid
		    LEFT JOIN char_jobs         j    ON j.charid    = c.charid
		    LEFT JOIN char_exp          e    ON e.charid    = c.charid
		    LEFT JOIN char_profile      pr   ON pr.charid   = c.charid
		    LEFT JOIN accounts_sessions sess ON sess.charid = c.charid
		    LEFT JOIN (
		        SELECT charid, SUM(job_points) AS total
		        FROM char_job_points GROUP BY charid
		    ) jpt ON jpt.charid = c.charid
		    WHERE ` + realCharacter + `
		) board
		WHERE value > 0
		ORDER BY value DESC, charname ASC`

		rows, err := db.sql.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("leaderboard %s: %w", m.Slug, err)
		}
		defer rows.Close()

		var out []LeaderRow
		for rows.Next() {
			var r LeaderRow
			if err := rows.Scan(&r.CharID, &r.Name, &r.Nation, &r.Race,
				&r.MainJob, &r.MainLvl, &r.Online, &r.Value); err != nil {
				return nil, fmt.Errorf("leaderboard %s scan: %w", m.Slug, err)
			}
			r.Rank = len(out) + 1
			out = append(out, r)
		}
		return out, rows.Err()
	})
}
