// Package web serves the game over HTTP: server-rendered templates with
// htmx partial swaps, cookie sessions, and JSON save files.
package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Directory walks skip _-prefixed files, so the partials get an explicit glob.
//
//go:embed templates templates/_*.html static
var assets embed.FS

const sessionCookie = "gogame_session"

// sessionMaxAge bounds how long a browser keeps its session cookie — and,
// paired with Store.Sweep, how long saves linger on disk.
const sessionMaxAge = 30 * 24 * time.Hour

// Session values are exactly 32 lowercase hex chars (128-bit crypto/rand).
var sessionRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Server routes requests against the session store.
type Server struct {
	store   *Store
	pages   map[string]*template.Template // page name -> parsed template set
	actionT *template.Template            // partial response for POST /game/action
}

// New builds the http.Server. The caller sets Addr before ListenAndServe.
func New(store *Store) *http.Server {
	s := &Server{store: store, pages: map[string]*template.Template{}}

	templates := mustSub(assets, "templates")
	static := mustSub(assets, "static")

	base := template.Must(template.New("layout.html").ParseFS(templates, "layout.html"))
	s.pages["title.html"] = withPage(base, templates, "title.html", "_gallery.html")
	s.pages["stats.html"] = withPage(base, templates, "stats.html")
	s.pages["game.html"] = withPage(base, templates, "game.html", "_scene.html", "_hand.html", "_log.html", "_gallery.html", "_deck.html")
	s.actionT = template.Must(template.New("action.html").ParseFS(templates, "_scene.html", "_hand.html", "_log.html", "_gallery.html", "_deck.html"))

	mux := http.NewServeMux()
	// no-cache keeps browsers honest during development: they must
	// revalidate static assets (Last-Modified/304) instead of serving
	// stale CSS/JS from an earlier build.
	mux.Handle("GET /static/", noCache(http.StripPrefix("/static/", http.FileServerFS(static))))
	mux.HandleFunc("GET /{$}", s.withSession(s.home))
	mux.HandleFunc("GET /stats", s.withSession(s.stats))
	mux.HandleFunc("POST /game/new", s.withSession(s.newGame))
	mux.HandleFunc("POST /game/action", s.withSession(s.action))
	return &http.Server{Handler: withLogging(mux)}
}

// statusRecorder captures the response status for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// withLogging logs non-static requests with status and duration.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// noCache forces revalidation of static assets so edits show up on reload.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func mustSub(fsys fs.FS, name string) fs.FS {
	sub, err := fs.Sub(fsys, name)
	if err != nil {
		panic(err) // embed paths are fixed at build time
	}
	return sub
}

// withPage clones the layout set and adds the page plus its dependencies.
func withPage(base *template.Template, templates fs.FS, pages ...string) *template.Template {
	t := template.Must(base.Clone())
	return template.Must(t.ParseFS(templates, pages...))
}

// withSession adapts a handler that needs the session ID. It reads the
// session cookie or creates one; the cookie is only set when newly created.
func (s *Server) withSession(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, s.session(w, r))
	}
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && sessionRe.MatchString(c.Value) {
		return c.Value
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return ""
	}
	v := hex.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    v,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return v
}
