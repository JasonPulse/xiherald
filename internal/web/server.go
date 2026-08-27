// Package web serves the Herald. Templates and static assets are embedded, so
// the binary is the whole deployment.
package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JasonPulse/xiherald/internal/xidb"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	db         *xidb.DB
	pages      map[string]*template.Template
	log        *slog.Logger
	serverName string
	portraits  *xidb.PortraitStore
	mux        *http.ServeMux
}

// assetVersions maps a static path to a short hash of its contents. Templates
// resolve asset URLs through it so that a changed file gets a changed URL.
//
// Without this, a long cache lifetime on /static/ is a trap: the paths are
// fixed, so a stylesheet edit stays invisible to anyone who has already loaded
// the old one until their cache expires. Content-addressed URLs are what make
// caching hard actually safe.
func assetVersions(root fs.FS) (map[string]string, error) {
	out := map[string]string{}

	err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		f, err := root.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		sum := sha256.New()
		if _, err := io.Copy(sum, f); err != nil {
			return err
		}

		out["/static/"+path] = hex.EncodeToString(sum.Sum(nil))[:10]
		return nil
	})

	return out, err
}

// pageTemplates are the top-level views. Each is parsed into its own template
// set alongside base.html, which is what lets every page define "content" and
// "sidebar" under the same names without colliding.
var pageTemplates = []string{
	"roster.html",
	"player.html",
	"stats.html",
	"leaderboard.html",
	"error.html",
}

// page is the envelope every template receives. Section drives which nav item
// is lit.
type page struct {
	ServerName string
	Title      string
	Section    string
	// PortraitHash is the hash of this character's stored portrait, empty when
	// none has been rendered. Templates emit an img tag only when it is set,
	// so an unrendered character costs no request rather than a 404 per view.
	PortraitHash string
	Data         any
}

func New(db *xidb.DB, serverName string, portraits *xidb.PortraitStore, log *slog.Logger) (*Server, error) {
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("static subtree: %w", err)
	}

	versions, err := assetVersions(static)
	if err != nil {
		return nil, fmt.Errorf("hash static assets: %w", err)
	}

	pages := make(map[string]*template.Template, len(pageTemplates))
	for _, name := range pageTemplates {
		set, err := template.New(name).Funcs(funcs(versions)).ParseFS(templateFS,
			"templates/base.html", "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		pages[name] = set
	}

	s := &Server{db: db, pages: pages, log: log, serverName: serverName,
		portraits: portraits, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(fmt.Sprintf("static subtree: %v", err))
	}

	s.mux.Handle("GET /static/", http.StripPrefix("/static/", staticCache(http.FileServer(http.FS(static)))))
	s.mux.HandleFunc("GET /livez", s.livez)
	s.mux.HandleFunc("GET /healthz", s.healthz)
	s.mux.HandleFunc("GET /{$}", s.roster)
	s.mux.HandleFunc("GET /player/{name}", s.player)
	s.mux.HandleFunc("GET /stats/{$}", s.statsIndex)
	s.mux.HandleFunc("GET /stats", s.statsIndex)
	s.mux.HandleFunc("GET /stats/{metric}", s.leaderboard)
	s.mux.HandleFunc("GET /portraits/{file}", s.portrait)

	s.apiRoutes()
}

// staticCache caches by whether the URL identifies its contents.
//
// A request carrying ?v= came from a template that resolved the file's content
// hash, so that URL can only ever mean one body and is safe to keep for a year.
// Anything else (a bare path, or a url() inside the stylesheet) gets minutes,
// so it cannot go stale for long.
func staticCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		next.ServeHTTP(w, r)
	})
}

// livez says only that the process is serving. It is the liveness probe, and
// it deliberately does not touch the database: the Herald tolerates the game
// database being down, so a database outage must not restart the pod.
func (s *Server) livez(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

// healthz reports whether the Herald can actually serve pages, which means the
// database has to answer. This is the readiness probe.
func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "database unreachable: %v\n", err)
		return
	}
	fmt.Fprintln(w, "ok")
}

type rosterData struct {
	Summary xidb.ServerSummary
	Rows    []xidb.RosterRow
	Sort    string
	Sorts   []struct{ Slug, Label string }
}

func (s *Server) roster(w http.ResponseWriter, r *http.Request) {
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "seen"
	}

	rows, err := s.db.Roster(r.Context(), sort)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	summary, err := s.db.Summary(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.render(w, r, "roster.html", page{
		Title:   "Roster",
		Section: "roster",
		Data: rosterData{
			Summary: summary,
			Rows:    rows,
			Sort:    sort,
			Sorts:   xidb.RosterSortOrder,
		},
	})
}

func (s *Server) player(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	p, err := s.db.Player(r.Context(), name)
	if errors.Is(err, xidb.ErrNotFound) {
		s.notFound(w, r, fmt.Sprintf("No character named %q walks Vana'diel.", name))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}

	pg := page{Title: p.Name, Section: "roster", Data: p}
	if hash, ok := s.portraits.HashFor(r.Context(), p.CharID); ok {
		pg.PortraitHash = hash
	}

	s.render(w, r, "player.html", pg)
}

type statsIndexData struct {
	Groups []struct {
		Name    string
		Metrics []xidb.Metric
	}
}

func (s *Server) statsIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "stats.html", page{
		Title:   "Fun stats",
		Section: "stats",
		Data:    statsIndexData{Groups: xidb.MetricGroups()},
	})
}

type leaderboardData struct {
	Metric xidb.Metric
	Rows   []xidb.LeaderRow
	Groups []struct {
		Name    string
		Metrics []xidb.Metric
	}
}

func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("metric")

	metric, ok := xidb.LookupMetric(slug)
	if !ok {
		s.notFound(w, r, fmt.Sprintf("No leaderboard called %q.", slug))
		return
	}

	rows, err := s.db.Leaderboard(r.Context(), metric)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.render(w, r, "leaderboard.html", page{
		Title:   metric.Label,
		Section: "stats",
		Data: leaderboardData{
			Metric: metric,
			Rows:   rows,
			Groups: xidb.MetricGroups(),
		},
	})
}

// portrait serves a rendered character image out of the portrait database.
//
// The path carries the character id and the query carries the appearance hash,
// so a re-rendered character gets a URL the browser has never seen. Without
// that a cached portrait would outlive the gear change that prompted it, which
// is the same trap the stylesheet fell into.
func (s *Server) portrait(w http.ResponseWriter, r *http.Request) {
	if !s.portraits.Enabled() {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimSuffix(r.PathValue("file"), ".png")
	charID, err := strconv.Atoi(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	img, err := s.portraits.Get(r.Context(), charID)
	if err != nil {
		s.log.Error("portrait read failed", "charid", charID, "err", err)
		http.Error(w, "portrait unavailable", http.StatusServiceUnavailable)
		return
	}
	if img == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("ETag", `"`+img.Hash+`"`)
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}

	http.ServeContent(w, r, name+".png", img.RenderedAt, bytesReader(img.Bytes))
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, p page) {
	p.ServerName = s.serverName

	set, ok := s.pages[name]
	if !ok {
		s.log.Error("unknown template", "template", name)
		http.Error(w, "template missing", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Pages must always be revalidated, otherwise a cached page keeps pointing
	// at the previous build's asset URLs and the fingerprinting buys nothing.
	w.Header().Set("Cache-Control", "no-cache")
	if err := set.ExecuteTemplate(w, "base.html", p); err != nil {
		// The response is already partly written by this point, so all that is
		// left is to record it.
		s.log.Error("render failed", "template", name, "path", r.URL.Path, "err", err)
	}
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request, message string) {
	w.WriteHeader(http.StatusNotFound)
	s.render(w, r, "error.html", page{
		Title:   "Not found",
		Section: "",
		Data:    message,
	})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "path", r.URL.Path, "err", err)
	w.WriteHeader(http.StatusInternalServerError)
	s.render(w, r, "error.html", page{
		Title:   "Error",
		Section: "",
		Data:    "The Herald could not reach the game database.",
	})
}

func funcs(versions map[string]string) template.FuncMap {
	return template.FuncMap{
		// asset turns a static path into a content-addressed URL.
		"asset": func(path string) string {
			if v, ok := versions[path]; ok {
				return path + "?v=" + v
			}
			return path
		},

		// comma groups thousands, which every number on this site needs.
		"comma": commaAny,

		"ratio": func(v float64) string { return fmt.Sprintf("%.2f", v) },

		"hours": func(v float64) string { return comma(int64(v)) },

		"skill": func(v float64) string { return fmt.Sprintf("%.1f", v) },

		"pct": func(v float64) string { return fmt.Sprintf("%.1f", v) },

		// value formats a leaderboard cell according to its metric.
		"value": func(f xidb.Format, v float64) string {
			switch f {
			case xidb.FormatRatio:
				return fmt.Sprintf("%.2f", v)
			case xidb.FormatHours:
				return comma(int64(v))
			default:
				return comma(int64(v))
			}
		},

		"date": func(t time.Time) string {
			if t.IsZero() {
				return "never"
			}
			return t.Format("2 Jan 2006")
		},

		"datetime": func(t time.Time) string {
			if t.IsZero() {
				return "never"
			}
			return t.Format("2 Jan 2006, 15:04")
		},

		// ago renders a coarse relative time. The Herald never needs seconds.
		"ago": func(t time.Time) string {
			if t.IsZero() {
				return "never"
			}
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				return plural(int(d.Minutes()), "min") + " ago"
			case d < 24*time.Hour:
				return plural(int(d.Hours()), "hr") + " ago"
			case d < 30*24*time.Hour:
				return plural(int(d.Hours()/24), "day") + " ago"
			default:
				return t.Format("2 Jan 2006")
			}
		},

		"lower": strings.ToLower,

		"add": func(a, b int) int { return a + b },
	}
}

// plural renders a count with its unit, singular at one.
func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// commaAny accepts whatever numeric type a template hands over. Templates
// carry ints from row structs and float64s from aggregate queries, and
// html/template will not convert between them.
func commaAny(v any) string {
	switch n := v.(type) {
	case int:
		return comma(int64(n))
	case int32:
		return comma(int64(n))
	case int64:
		return comma(n)
	case uint:
		return comma(int64(n))
	case uint64:
		return comma(int64(n))
	case float32:
		return comma(int64(n))
	case float64:
		return comma(int64(n))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// comma inserts thousands separators. Written out rather than pulled from a
// dependency because it is six lines and the Herald has one dependency.
func comma(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}

	digits := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}

	if negative {
		return "-" + b.String()
	}
	return b.String()
}

// bytesReader adapts a blob to what http.ServeContent needs, which gives range
// requests and conditional GETs for free.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
