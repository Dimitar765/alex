package web

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"goGame/internal/content"
	"goGame/internal/game"
)

// startScene is the content contract for the entry scene (validated by
// content.Load).
const startScene = "title"

type choiceView struct {
	Index     int
	Text      string
	Requires  string
	Available bool
}

type cardView struct {
	ID   string
	Name string
	Text string
}

// view is the template data for the game page and the action partial.
type view struct {
	Stats   game.Stats
	Scene   content.Scene
	Choices []choiceView
	Hand    []cardView
	Log     []string
}

func (s *Server) home(w http.ResponseWriter, r *http.Request, session string) {
	st := s.store.Get(session)
	if st == nil {
		s.renderPage(w, "title.html", s.store.lib.Scenes[startScene])
		return
	}
	s.renderPage(w, "game.html", s.buildView(st, nil))
}

func (s *Server) newGame(w http.ResponseWriter, r *http.Request, session string) {
	st := game.NewState(s.store.lib.CardList(), startScene)
	st.Session = session
	if err := s.store.Put(st); err != nil {
		http.Error(w, "could not save game", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) action(w http.ResponseWriter, r *http.Request, session string) {
	next, err := s.store.Update(session, func(st *game.State) error {
		if scene := s.store.lib.Scenes[st.SceneID]; scene.Ending != "" {
			return errors.New("the campaign is over — start a new game")
		}
		switch {
		case r.FormValue("card") != "":
			return st.PlayCard(r.FormValue("card"), s.store.lib.Cards)
		case r.FormValue("choice") != "":
			return s.applyChoice(st, r.FormValue("choice"))
		default:
			return errors.New("no card or choice selected")
		}
	})
	if next == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.respondPartial(w, s.buildView(next, err))
}

func (s *Server) applyChoice(st *game.State, idx string) error {
	n, err := strconv.Atoi(idx)
	if err != nil {
		return errors.New("malformed choice")
	}
	scene := s.store.lib.Scenes[st.SceneID]
	if n < 0 || n >= len(scene.Choices) {
		return errors.New("no such choice")
	}
	return st.Choose(scene.Choices[n], s.store.lib.Cards)
}

// buildView projects state into template data. A non-nil engine error is
// appended to the rendered log only; the persisted state stays untouched.
func (s *Server) buildView(st *game.State, err error) view {
	scene := s.store.lib.Scenes[st.SceneID]
	v := view{Stats: st.Stats, Scene: scene, Log: slices.Clone(st.Log)}
	for i, c := range scene.Choices {
		var reqs []string
		if c.RequiresCard != "" {
			reqs = append(reqs, s.store.lib.Cards[c.RequiresCard].Name)
		}
		for _, k := range slices.Sorted(maps.Keys(c.RequiresStat)) {
			reqs = append(reqs, fmt.Sprintf("%s %d", game.StatLabel(k), c.RequiresStat[k]))
		}
		v.Choices = append(v.Choices, choiceView{
			Index:     i,
			Text:      c.Text,
			Requires:  strings.Join(reqs, " · "),
			Available: st.CanChoose(c),
		})
	}
	for _, id := range st.Hand {
		v.Hand = append(v.Hand, cardView{ID: id, Name: s.store.lib.Cards[id].Name, Text: s.store.lib.Cards[id].Text})
	}
	if err != nil {
		v.Log = append(v.Log, "Error: "+err.Error())
	}
	return v
}

func (s *Server) renderPage(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pages[page].ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

// respondPartial renders the three swap targets: #scene inline (the htmx
// target) plus #hand and #log as out-of-band swaps.
func (s *Server) respondPartial(w http.ResponseWriter, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.actionT.ExecuteTemplate(w, "actionResponse", v); err != nil {
		log.Printf("render partials: %v", err)
	}
}
