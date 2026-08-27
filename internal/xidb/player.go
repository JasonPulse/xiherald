package xidb

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Player is everything the character detail page shows.
type Player struct {
	RosterRow

	Gil      int
	Merits   int
	Limits   int
	Exemplar int

	RankPoints int
	Ranks      []NationRank
	Fame       []FameEntry

	Jobs    []JobRow
	Skills  []SkillBucket
	History []HistoryStat
	Crafts  []CraftRow
}

type NationRank struct {
	Nation string
	Slug   string
	Rank   int
}

type FameEntry struct {
	Area  string
	Value int
}

type JobRow struct {
	ID             int
	Short          string
	Name           string
	Level          int
	Exp            int
	IsMain         bool
	IsSub          bool
	JobPoints      int
	JobPointsSpent int
	Capacity       int
}

// Capped reports whether the job has reached the level-99 cap, which is what
// the roster's "jobs at 99" count keys off.
func (j JobRow) Capped() bool { return j.Level >= 99 }

// LevelPercent drives the job level bar.
func (j JobRow) LevelPercent() float64 {
	if j.Level <= 0 {
		return 0
	}
	if j.Level > 99 {
		return 100
	}
	return float64(j.Level) / 99.0 * 100.0
}

type SkillRow struct {
	ID    int
	Name  string
	Value int // stored at ten times the displayed skill
	Rank  int
	Cap   int // also stored at ten times, 0 when unknown
}

// Display is the skill number as the game shows it. Combat and magic skill
// read as whole numbers even though the database keeps a tenths digit.
func (s SkillRow) Display() int { return s.Value / 10 }

// CapDisplay is the cap as the game shows it.
func (s SkillRow) CapDisplay() int { return s.Cap / 10 }

func (s SkillRow) Percent() float64 {
	if s.Cap <= 0 {
		return 0
	}
	if s.Value >= s.Cap {
		return 100
	}
	return float64(s.Value) / float64(s.Cap) * 100.0
}

func (s SkillRow) AtCap() bool { return s.Cap > 0 && s.Value >= s.Cap }

type SkillBucket struct {
	Group SkillGroup
	Rows  []SkillRow
}

type CraftRow struct {
	Name   string
	Value  int
	Rank   string
	Points int
}

func (c CraftRow) Display() float64 { return float64(c.Value) / 10.0 }

// HistoryStat is one char_history counter, already labelled for display.
type HistoryStat struct {
	Label string
	Value int
	Unit  string
}

// craftSkills pairs a craft skill id with its char_points guild point column.
// Clothcraft's points column is named after weaving, which is the one place
// the two vocabularies disagree.
var craftSkills = []struct {
	SkillID int
	Points  string
}{
	{48, "guild_fishing"},
	{49, "guild_woodworking"},
	{50, "guild_smithing"},
	{51, "guild_goldsmithing"},
	{52, "guild_weaving"},
	{53, "guild_leathercraft"},
	{54, "guild_bonecraft"},
	{55, "guild_alchemy"},
	{56, "guild_cooking"},
}

var fameAreas = []struct {
	Column string
	Label  string
}{
	{"fame_sandoria", "San d'Oria"},
	{"fame_bastok", "Bastok"},
	{"fame_windurst", "Windurst"},
	{"fame_jeuno", "Jeuno"},
	{"fame_norg", "Norg"},
	{"fame_adoulin", "Adoulin"},
	{"fame_aby_konschtat", "Abyssea - Konschtat"},
	{"fame_aby_tahrongi", "Abyssea - Tahrongi"},
	{"fame_aby_latheine", "Abyssea - La Theine"},
	{"fame_aby_misareaux", "Abyssea - Misareaux"},
	{"fame_aby_vunkerl", "Abyssea - Vunkerl"},
	{"fame_aby_attohwa", "Abyssea - Attohwa"},
	{"fame_aby_altepa", "Abyssea - Altepa"},
	{"fame_aby_grauberg", "Abyssea - Grauberg"},
	{"fame_aby_uleguerand", "Abyssea - Uleguerand"},
}

// Player loads one character by name. Names are unique in chars, and the
// lookup is case-insensitive because the collation is.
func (db *DB) Player(ctx context.Context, name string) (*Player, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrNotFound
	}

	return cached(db.cache, "player:"+strings.ToLower(name), func() (*Player, error) {
		p := &Player{}

		if err := db.loadIdentity(ctx, name, p); err != nil {
			return nil, err
		}
		if err := db.loadProfile(ctx, p); err != nil {
			return nil, err
		}
		if err := db.loadJobs(ctx, p); err != nil {
			return nil, err
		}
		if err := db.loadSkills(ctx, p); err != nil {
			return nil, err
		}
		if err := db.loadHistory(ctx, p); err != nil {
			return nil, err
		}
		return p, nil
	})
}

func (db *DB) loadIdentity(ctx context.Context, name string, p *Player) error {
	totalExpr, cappedExpr := jobSum()

	query := `SELECT` + rosterColumns + `,
	  ` + totalExpr + `  AS total_levels,
	  ` + cappedExpr + ` AS jobs_capped,
	  sess.charid IS NOT NULL AS online,
	  COALESCE(gil.quantity, 0)   AS gil,
	  COALESCE(e.merits, 0)       AS merits,
	  COALESCE(e.limits, 0)       AS limits,
	  COALESCE(s.exemplar_points, 0) AS exemplar` +
		rosterJoins + `
	LEFT JOIN char_exp e ON e.charid = c.charid
	LEFT JOIN char_inventory gil
	       ON gil.charid = c.charid AND gil.location = 0 AND gil.slot = 0
	WHERE c.charname = ? AND ` + realCharacter + `
	LIMIT 1`

	r := &p.RosterRow
	err := db.sql.QueryRowContext(ctx, query, name).Scan(
		&r.CharID, &r.Name, &r.Nation, &r.Playtime, &r.Created, &r.LastLogout,
		&r.ZoneID, &r.ZoneName, &r.Race, &r.MainJob, &r.MainLevel, &r.SubJob,
		&r.SubLevel, &r.MasterLevel, &r.TitleID, &r.Kills, &r.Deaths,
		&r.TotalLevels, &r.JobsCapped, &r.Online,
		&p.Gil, &p.Merits, &p.Limits, &p.Exemplar,
	)
	if isNoRows(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("player identity: %w", err)
	}
	return nil
}

func (db *DB) loadProfile(ctx context.Context, p *Player) error {
	cols := []string{"rank_points", "rank_sandoria", "rank_bastok", "rank_windurst"}
	for _, f := range fameAreas {
		cols = append(cols, f.Column)
	}

	query := `SELECT ` + strings.Join(cols, ", ") +
		` FROM char_profile WHERE charid = ?`

	values := make([]int, len(cols))
	targets := make([]any, len(cols))
	for i := range values {
		targets[i] = &values[i]
	}

	err := db.sql.QueryRowContext(ctx, query, p.CharID).Scan(targets...)
	if isNoRows(err) {
		// A character who has never been given a profile row still renders.
		return nil
	}
	if err != nil {
		return fmt.Errorf("player profile: %w", err)
	}

	p.RankPoints = values[0]
	p.Ranks = []NationRank{
		{Nation: NationName(0), Slug: NationSlug(0), Rank: values[1]},
		{Nation: NationName(1), Slug: NationSlug(1), Rank: values[2]},
		{Nation: NationName(2), Slug: NationSlug(2), Rank: values[3]},
	}
	for i, f := range fameAreas {
		if v := values[4+i]; v > 0 {
			p.Fame = append(p.Fame, FameEntry{Area: f.Label, Value: v})
		}
	}
	return nil
}

func (db *DB) loadJobs(ctx context.Context, p *Player) error {
	levelCols := make([]string, 0, len(JobColumns))
	expCols := make([]string, 0, len(JobColumns))
	for _, col := range JobColumns {
		levelCols = append(levelCols, "COALESCE(j."+col+",0)")
		expCols = append(expCols, "COALESCE(e."+col+",0)")
	}

	query := `SELECT ` + strings.Join(levelCols, ", ") + `, ` +
		strings.Join(expCols, ", ") + `
	FROM chars c
	LEFT JOIN char_jobs j ON j.charid = c.charid
	LEFT JOIN char_exp  e ON e.charid = c.charid
	WHERE c.charid = ?`

	n := len(JobColumns)
	values := make([]int, n*2)
	targets := make([]any, n*2)
	for i := range values {
		targets[i] = &values[i]
	}

	if err := db.sql.QueryRowContext(ctx, query, p.CharID).Scan(targets...); err != nil {
		if isNoRows(err) {
			return nil
		}
		return fmt.Errorf("player jobs: %w", err)
	}

	// Job points are one row per job, so they are collected separately and
	// merged in.
	type jp struct{ points, spent, capacity int }
	byJob := map[int]jp{}
	rows, err := db.sql.QueryContext(ctx,
		`SELECT jobid, job_points, job_points_spent, capacity_points
		 FROM char_job_points WHERE charid = ?`, p.CharID)
	if err != nil {
		return fmt.Errorf("player job points: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, points, spent, capacity int
		if err := rows.Scan(&id, &points, &spent, &capacity); err != nil {
			return fmt.Errorf("player job points scan: %w", err)
		}
		byJob[id] = jp{points, spent, capacity}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		jobID := i + 1
		row := JobRow{
			ID:     jobID,
			Short:  JobShort(jobID),
			Name:   JobFull(jobID),
			Level:  values[i],
			Exp:    values[n+i],
			IsMain: jobID == p.MainJob,
			IsSub:  jobID == p.SubJob,
		}
		if v, ok := byJob[jobID]; ok {
			row.JobPoints = v.points
			row.JobPointsSpent = v.spent
			row.Capacity = v.capacity
		}
		p.Jobs = append(p.Jobs, row)
	}
	return nil
}

func (db *DB) loadSkills(ctx context.Context, p *Player) error {
	// skill_caps stores one column per skill rank, so the cap for a given
	// skill is picked out by that skill's own rank at the character's level.
	branches := make([]string, 0, 14)
	for r := 0; r < 14; r++ {
		branches = append(branches, fmt.Sprintf("WHEN %d THEN COALESCE(sc.r%d, 0)", r, r))
	}

	query := `
	SELECT k.skillid, k.value, k.rank,
	       CASE k.rank ` + strings.Join(branches, " ") + ` ELSE 0 END AS cap
	FROM char_skills k
	LEFT JOIN char_stats s  ON s.charid = k.charid
	LEFT JOIN skill_caps sc ON sc.level = COALESCE(s.mlvl, 1)
	WHERE k.charid = ?
	ORDER BY k.skillid`

	rows, err := db.sql.QueryContext(ctx, query, p.CharID)
	if err != nil {
		return fmt.Errorf("player skills: %w", err)
	}
	defer rows.Close()

	byGroup := map[SkillGroup][]SkillRow{}
	craftValues := map[int]int{}

	for rows.Next() {
		var s SkillRow
		if err := rows.Scan(&s.ID, &s.Value, &s.Rank, &s.Cap); err != nil {
			return fmt.Errorf("player skills scan: %w", err)
		}
		name, known := SkillName(s.ID)
		if !known || s.Value == 0 {
			continue
		}
		s.Name = name

		group := SkillGroupOf(s.ID)
		if group == SkillCraft {
			// Craft skill is not gated by character level; it caps at the
			// guild ceiling, and skill_caps has nothing to say about it.
			craftValues[s.ID] = s.Value
			continue
		}
		// skill_caps holds plain skill numbers while char_skills stores them
		// at ten times, so the cap is scaled to match before comparison.
		s.Cap *= 10
		byGroup[group] = append(byGroup[group], s)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, g := range SkillGroupOrder {
		if rows := byGroup[g]; len(rows) > 0 {
			p.Skills = append(p.Skills, SkillBucket{Group: g, Rows: rows})
		}
	}

	return db.loadCrafts(ctx, p, craftValues)
}

func (db *DB) loadCrafts(ctx context.Context, p *Player, values map[int]int) error {
	cols := make([]string, 0, len(craftSkills))
	for _, c := range craftSkills {
		cols = append(cols, c.Points)
	}

	points := make([]int, len(cols))
	targets := make([]any, len(cols))
	for i := range points {
		targets[i] = &points[i]
	}

	err := db.sql.QueryRowContext(ctx,
		`SELECT `+strings.Join(cols, ", ")+` FROM char_points WHERE charid = ?`,
		p.CharID).Scan(targets...)
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("player crafts: %w", err)
	}

	for i, c := range craftSkills {
		value := values[c.SkillID]
		if value == 0 && points[i] == 0 {
			continue
		}
		name, _ := SkillName(c.SkillID)
		p.Crafts = append(p.Crafts, CraftRow{
			Name:   name,
			Value:  value,
			Rank:   CraftRank(value),
			Points: points[i],
		})
	}
	return nil
}

func (db *DB) loadHistory(ctx context.Context, p *Player) error {
	values := make([]int, len(HistoryColumns))
	targets := make([]any, len(HistoryColumns))
	cols := make([]string, len(HistoryColumns))
	for i, h := range HistoryColumns {
		cols[i] = h.Column
		targets[i] = &values[i]
	}

	err := db.sql.QueryRowContext(ctx,
		`SELECT `+strings.Join(cols, ", ")+` FROM char_history WHERE charid = ?`,
		p.CharID).Scan(targets...)
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("player history: %w", err)
	}

	for i, h := range HistoryColumns {
		p.History = append(p.History, HistoryStat{
			Label: h.Label, Value: values[i], Unit: h.Unit,
		})
	}
	return nil
}

// PlaytimeText renders chars.playtime as days and hours.
func (p *Player) PlaytimeText() string {
	d := time.Duration(p.Playtime) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
}
