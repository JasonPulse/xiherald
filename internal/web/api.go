package web

// JSON API for other tools in the cluster.
//
// The response types here are deliberately separate from the xidb structs. They
// are a published contract: renaming a Go field or reordering a query must not
// silently change somebody else's website. Every field is explicitly mapped and
// explicitly tagged.
//
// Gil is not exposed. The known consumer is a public-facing guild site, and a
// character's gil balance is the one number on the Herald that turns into a
// griefing target the moment it leaves a private network. It is still on the
// HTML character page, which is private. If a caller genuinely needs it, that
// should be a deliberate decision rather than a field somebody finds by
// accident.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/JasonPulse/xiherald/internal/xidb"
)

// apiVersion is the path segment. Bump it for a breaking change rather than
// altering the meaning of an existing field.
const apiVersion = "v1"

type apiIndex struct {
	Server    string            `json:"server"`
	Version   string            `json:"version"`
	Endpoints map[string]string `json:"endpoints"`
	Notes     []string          `json:"notes"`
}

type apiSummary struct {
	Characters  int       `json:"characters"`
	Online      int       `json:"online"`
	TotalKills  int       `json:"total_kills"`
	TotalDeaths int       `json:"total_deaths"`
	TotalHours  float64   `json:"total_hours"`
	JobsAt99    int       `json:"jobs_at_99"`
	NewestChar  string    `json:"newest_character"`
	NewestAt    time.Time `json:"newest_character_created"`
}

type apiCharacter struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Nation   string `json:"nation"`
	NationID int    `json:"nation_id"`
	Race     string `json:"race"`
	Sex      string `json:"sex"`
	Title    string `json:"title"`

	MainJob      string `json:"main_job"`
	MainJobLevel int    `json:"main_job_level"`
	SubJob       string `json:"sub_job"`
	SubJobLevel  int    `json:"sub_job_level"`
	MasterLevel  int    `json:"master_level"`

	JobsAt99       int     `json:"jobs_at_99"`
	TotalJobLevels int     `json:"total_job_levels"`
	Kills          int     `json:"kills"`
	Deaths         int     `json:"deaths"`
	KD             float64 `json:"kill_death_ratio"`

	PlaytimeSeconds int    `json:"playtime_seconds"`
	Zone            string `json:"zone"`
	ZoneID          int    `json:"zone_id"`
	Online          bool   `json:"online"`

	Created    time.Time `json:"created"`
	LastLogout time.Time `json:"last_logout"`
}

type apiJob struct {
	ID             int    `json:"id"`
	Abbrev         string `json:"abbrev"`
	Name           string `json:"name"`
	Level          int    `json:"level"`
	Exp            int    `json:"exp"`
	JobPoints      int    `json:"job_points"`
	JobPointsSpent int    `json:"job_points_spent"`
	CapacityPoints int    `json:"capacity_points"`
	IsMain         bool   `json:"is_main"`
	IsSub          bool   `json:"is_sub"`
}

type apiSkill struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	Value int    `json:"value"`
	Cap   int    `json:"cap"`
	Rank  int    `json:"rank"`
	AtCap bool   `json:"at_cap"`
}

type apiCraft struct {
	Name        string  `json:"name"`
	Skill       float64 `json:"skill"`
	Rank        string  `json:"rank"`
	GuildPoints int     `json:"guild_points"`
}

type apiNationRank struct {
	Nation string `json:"nation"`
	Rank   int    `json:"rank"`
}

type apiFame struct {
	Area  string `json:"area"`
	Value int    `json:"value"`
}

type apiCharacterDetail struct {
	apiCharacter

	Merits         int `json:"merits"`
	LimitPoints    int `json:"limit_points"`
	ExemplarPoints int `json:"exemplar_points"`
	RankPoints     int `json:"rank_points"`

	NationRanks []apiNationRank `json:"nation_ranks"`
	Fame        []apiFame       `json:"fame"`
	Jobs        []apiJob        `json:"jobs"`
	Skills      []apiSkill      `json:"skills"`
	Crafts      []apiCraft      `json:"crafts"`
	History     map[string]int  `json:"history"`
}

type apiMetric struct {
	Slug   string `json:"slug"`
	Label  string `json:"label"`
	Blurb  string `json:"description"`
	Group  string `json:"group"`
	Unit   string `json:"unit,omitempty"`
	Format string `json:"format"`
}

type apiLeaderRow struct {
	Rank         int     `json:"rank"`
	CharacterID  int     `json:"character_id"`
	Name         string  `json:"name"`
	Nation       string  `json:"nation"`
	MainJob      string  `json:"main_job"`
	MainJobLevel int     `json:"main_job_level"`
	Online       bool    `json:"online"`
	Value        float64 `json:"value"`
}

type apiLeaderboard struct {
	Metric apiMetric      `json:"metric"`
	Rows   []apiLeaderRow `json:"rows"`
}

func toAPICharacter(r xidb.RosterRow) apiCharacter {
	return apiCharacter{
		ID:              r.CharID,
		Name:            r.Name,
		Nation:          r.NationName(),
		NationID:        r.Nation,
		Race:            r.RaceName(),
		Sex:             r.RaceSex(),
		Title:           r.Title(),
		MainJob:         xidb.JobShort(r.MainJob),
		MainJobLevel:    r.MainLevel,
		SubJob:          xidb.JobShort(r.SubJob),
		SubJobLevel:     r.SubLevel,
		MasterLevel:     r.MasterLevel,
		JobsAt99:        r.JobsCapped,
		TotalJobLevels:  r.TotalLevels,
		Kills:           r.Kills,
		Deaths:          r.Deaths,
		KD:              r.KD(),
		PlaytimeSeconds: r.Playtime,
		Zone:            r.Zone(),
		ZoneID:          r.ZoneID,
		Online:          r.Online,
		Created:         r.Created,
		LastLogout:      r.LastLogout,
	}
}

func toAPIMetric(m xidb.Metric) apiMetric {
	return apiMetric{
		Slug:   m.Slug,
		Label:  m.Label,
		Blurb:  m.Blurb,
		Group:  m.Group,
		Unit:   m.Unit,
		Format: string(m.Format),
	}
}

func (s *Server) apiRoutes() {
	base := "/api/" + apiVersion

	s.mux.HandleFunc("GET "+base, s.apiIndex)
	s.mux.HandleFunc("GET "+base+"/{$}", s.apiIndex)
	s.mux.HandleFunc("GET "+base+"/summary", s.apiSummary)
	s.mux.HandleFunc("GET "+base+"/characters", s.apiCharacters)
	s.mux.HandleFunc("GET "+base+"/characters/{name}", s.apiCharacter)
	s.mux.HandleFunc("GET "+base+"/leaderboards", s.apiMetrics)
	s.mux.HandleFunc("GET "+base+"/leaderboards/{metric}", s.apiLeaderboard)
}

// writeJSON is the single place a JSON response is produced, so the headers
// cannot drift between endpoints. The body is built before anything is written
// so a marshalling failure can still return a 500 rather than truncated JSON.
func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		s.log.Error("api marshal failed", "path", r.URL.Path, "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(status)
	w.Write(append(body, '\n'))
}

func (s *Server) apiError(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.writeJSON(w, r, status, map[string]string{"error": message})
}

func (s *Server) apiIndex(w http.ResponseWriter, r *http.Request) {
	base := "/api/" + apiVersion

	s.writeJSON(w, r, http.StatusOK, apiIndex{
		Server:  s.serverName,
		Version: apiVersion,
		Endpoints: map[string]string{
			"summary":      base + "/summary",
			"characters":   base + "/characters",
			"character":    base + "/characters/{name}",
			"leaderboards": base + "/leaderboards",
			"leaderboard":  base + "/leaderboards/{metric}",
		},
		Notes: []string{
			"Read-only. Everything comes from the game database as the server recorded it.",
			"Gil is not exposed by this API.",
			"Characters with a blank name are unfinished slots and are never listed.",
			"deaths is char_history.times_knocked_out; char_stats.death is a weakness timer, not a tally.",
			"Skill values and caps are the numbers the game displays. Craft skill carries one decimal.",
		},
	})
}

func (s *Server) apiSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := s.db.Summary(r.Context())
	if err != nil {
		s.log.Error("api summary failed", "err", err)
		s.apiError(w, r, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	s.writeJSON(w, r, http.StatusOK, apiSummary{
		Characters:  sum.Characters,
		Online:      sum.Online,
		TotalKills:  sum.TotalKills,
		TotalDeaths: sum.TotalDeaths,
		TotalHours:  sum.TotalHours,
		JobsAt99:    sum.JobsAt99,
		NewestChar:  sum.NewestChar,
		NewestAt:    sum.NewestJoined,
	})
}

func (s *Server) apiCharacters(w http.ResponseWriter, r *http.Request) {
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "name"
	}

	rows, err := s.db.Roster(r.Context(), sort)
	if err != nil {
		s.log.Error("api characters failed", "err", err)
		s.apiError(w, r, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	// Always an array, never null: a consumer iterating the result should not
	// have to special-case an empty server.
	out := make([]apiCharacter, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPICharacter(row))
	}

	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"count":      len(out),
		"characters": out,
	})
}

func (s *Server) apiCharacter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	p, err := s.db.Player(r.Context(), name)
	if errors.Is(err, xidb.ErrNotFound) {
		s.apiError(w, r, http.StatusNotFound, fmt.Sprintf("no character named %q", name))
		return
	}
	if err != nil {
		s.log.Error("api character failed", "err", err)
		s.apiError(w, r, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	detail := apiCharacterDetail{
		apiCharacter:   toAPICharacter(p.RosterRow),
		Merits:         p.Merits,
		LimitPoints:    p.Limits,
		ExemplarPoints: p.Exemplar,
		RankPoints:     p.RankPoints,
		NationRanks:    make([]apiNationRank, 0, len(p.Ranks)),
		Fame:           make([]apiFame, 0, len(p.Fame)),
		Jobs:           make([]apiJob, 0, len(p.Jobs)),
		Skills:         make([]apiSkill, 0),
		Crafts:         make([]apiCraft, 0, len(p.Crafts)),
		History:        map[string]int{},
	}

	for _, n := range p.Ranks {
		detail.NationRanks = append(detail.NationRanks, apiNationRank{Nation: n.Nation, Rank: n.Rank})
	}
	for _, f := range p.Fame {
		detail.Fame = append(detail.Fame, apiFame{Area: f.Area, Value: f.Value})
	}
	for _, j := range p.Jobs {
		detail.Jobs = append(detail.Jobs, apiJob{
			ID: j.ID, Abbrev: j.Short, Name: j.Name, Level: j.Level, Exp: j.Exp,
			JobPoints: j.JobPoints, JobPointsSpent: j.JobPointsSpent,
			CapacityPoints: j.Capacity, IsMain: j.IsMain, IsSub: j.IsSub,
		})
	}
	// Skills are grouped for the HTML page; the API flattens them and names the
	// group on each row, which is easier for a consumer to filter.
	for _, bucket := range p.Skills {
		for _, sk := range bucket.Rows {
			detail.Skills = append(detail.Skills, apiSkill{
				ID: sk.ID, Name: sk.Name, Group: string(bucket.Group),
				Value: sk.Display(), Cap: sk.CapDisplay(), Rank: sk.Rank,
				AtCap: sk.AtCap(),
			})
		}
	}
	for _, c := range p.Crafts {
		detail.Crafts = append(detail.Crafts, apiCraft{
			Name: c.Name, Skill: c.Display(), Rank: c.Rank, GuildPoints: c.Points,
		})
	}
	for i, h := range p.History {
		detail.History[xidb.HistoryColumns[i].Column] = h.Value
	}

	s.writeJSON(w, r, http.StatusOK, detail)
}

func (s *Server) apiMetrics(w http.ResponseWriter, r *http.Request) {
	out := make([]apiMetric, 0, len(xidb.Metrics))
	for _, m := range xidb.Metrics {
		out = append(out, toAPIMetric(m))
	}

	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"count":        len(out),
		"leaderboards": out,
	})
}

func (s *Server) apiLeaderboard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("metric")

	metric, ok := xidb.LookupMetric(slug)
	if !ok {
		s.apiError(w, r, http.StatusNotFound, fmt.Sprintf("no leaderboard named %q", slug))
		return
	}

	rows, err := s.db.Leaderboard(r.Context(), metric)
	if err != nil {
		s.log.Error("api leaderboard failed", "err", err)
		s.apiError(w, r, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	out := make([]apiLeaderRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiLeaderRow{
			Rank: row.Rank, CharacterID: row.CharID, Name: row.Name,
			Nation: row.NationName(), MainJob: xidb.JobShort(row.MainJob),
			MainJobLevel: row.MainLvl, Online: row.Online, Value: row.Value,
		})
	}

	s.writeJSON(w, r, http.StatusOK, apiLeaderboard{
		Metric: toAPIMetric(metric),
		Rows:   out,
	})
}
