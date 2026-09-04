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

type apiEquipPiece struct {
	Slot   string `json:"slot"`
	Label  string `json:"label"`
	ItemID int    `json:"item_id"`
	Name   string `json:"name"`
}

type apiLoadout struct {
	JobID     int             `json:"job_id"`
	Job       string          `json:"job"`
	JobName   string          `json:"job_name"`
	Pieces    []apiEquipPiece `json:"pieces"`
	OtherJobs []string        `json:"other_jobs"`
}

type apiMission struct {
	Log            string `json:"log"`
	LogID          int    `json:"log_id"`
	Short          string `json:"short"`
	CurrentID      int    `json:"current_id,omitempty"`
	Current        string `json:"current,omitempty"`
	HasCurrent     bool   `json:"has_current"`
	LastCompleteID int    `json:"last_complete_id,omitempty"`
	LastComplete   string `json:"last_complete,omitempty"`
	Completed      int    `json:"completed"`
	Total          int    `json:"total"`
	Finished       bool   `json:"finished"`
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
	Appearance  *apiAppearance  `json:"appearance,omitempty"`
	Gear        *apiLoadout     `json:"gear,omitempty"`
	Missions    []apiMission    `json:"missions"`
	History     map[string]int  `json:"history"`
}

// apiAppearance is the render work-list entry. models is keyed by slot name so
// a renderer does not have to know the positional order, and equip_arg is the
// same data pre-joined into the form Vellichor's VELLICHOR_PC_EQUIP takes.
type apiAppearance struct {
	CharacterID int            `json:"character_id"`
	Name        string         `json:"name"`
	Race        int            `json:"race"`
	RaceName    string         `json:"race_name"`
	Face        int            `json:"face"`
	Size        int            `json:"size"`
	StyleLocked bool           `json:"style_locked"`
	Models      map[string]int `json:"models"`
	EquipArg    string         `json:"equip_arg"`
	Hash        string         `json:"hash"`
	Renderable  bool           `json:"renderable"`
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
	s.mux.HandleFunc("GET "+base+"/appearances", s.apiAppearances)
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
			"Mission has_current is authoritative: the no-mission sentinel is 65535 for nation logs and 0 for expansion logs.",
			"Appearance models are resolved: char_look holds model ids, char_style holds item ids mapped through item_equipment.MId when style-locked.",
			"Poll appearances and re-render only where hash changed.",
			"Gear comes from char_equip_saved for the character's current main job, so it is the last set saved for that job rather than the live one.",
			"Item names lose apostrophes: the database stores them stripped with no case seam to recover, so Kirin's Osode reads as Kirins Osode.",
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
		Missions:       make([]apiMission, 0, len(p.Missions)),
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
	// Every log is reported, including untouched ones, because a consumer
	// filtering for "has this character finished CoP" should not have to infer
	// absence from a missing key.
	for _, m := range p.Missions {
		detail.Missions = append(detail.Missions, apiMission{
			Log: m.Log.Label, LogID: m.Log.LogID, Short: m.Log.Short,
			CurrentID: m.CurrentID, Current: m.Current, HasCurrent: m.HasCurrent,
			LastCompleteID: m.LastDoneID, LastComplete: m.LastDone,
			Completed: m.Completed, Total: m.Total, Finished: m.Finished(),
		})
	}
	for i, h := range p.History {
		detail.History[xidb.HistoryColumns[i].Column] = h.Value
	}

	if !p.Gear.Empty() {
		gear := apiLoadout{
			JobID: p.Gear.JobID, Job: p.Gear.Job, JobName: p.Gear.JobName,
			Pieces:    make([]apiEquipPiece, 0, len(p.Gear.Pieces)),
			OtherJobs: p.Gear.OtherJobs,
		}
		for _, pc := range p.Gear.Pieces {
			gear.Pieces = append(gear.Pieces, apiEquipPiece{
				Slot: pc.Slot, Label: pc.Label, ItemID: pc.ItemID, Name: pc.Name,
			})
		}
		if gear.OtherJobs == nil {
			gear.OtherJobs = []string{}
		}
		detail.Gear = &gear
	}

	if look, err := s.db.AppearanceOf(r.Context(), p.Name); err == nil {
		a := toAPIAppearance(look)
		detail.Appearance = &a
	}

	s.writeJSON(w, r, http.StatusOK, detail)
}

func toAPIAppearance(a xidb.Appearance) apiAppearance {
	models := make(map[string]int, len(xidb.AppearanceSlots))
	for i, slot := range xidb.AppearanceSlots {
		models[slot] = a.Models[i]
	}

	return apiAppearance{
		CharacterID: a.CharID,
		Name:        a.Name,
		Race:        a.Race,
		RaceName:    a.RaceName(),
		Face:        a.Face,
		Size:        a.Size,
		StyleLocked: a.StyleLocked,
		Models:      models,
		EquipArg:    a.EquipArg(),
		Hash:        a.Hash(),
		Renderable:  a.Renderable(),
	}
}

func (s *Server) apiAppearances(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.Appearances(r.Context())
	if err != nil {
		s.log.Error("api appearances failed", "err", err)
		s.apiError(w, r, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	out := make([]apiAppearance, 0, len(list))
	for _, a := range list {
		out = append(out, toAPIAppearance(a))
	}

	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"count":       len(out),
		"appearances": out,
	})
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
