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
	ID       string
	Name     string
	Text     string
	Cost     int
	Playable bool
}

// endingTile is one slot in the endings gallery.
type endingTile struct {
	Class      string
	Discovered bool
}

// view is the template data for the game page and the action partial.
type view struct {
	Stats       game.Stats
	Scene       content.Scene
	Choices     []choiceView
	Hand        []cardView
	Log         []string
	Turns       int
	CardsPlayed int
	Campaigns   int
	Endings     []endingTile
}

// titleView is the template data for the title page.
type titleView struct {
	Scene     content.Scene
	Campaigns int
	Endings   []endingTile
}

// gallery builds the endings tiles over content.EndingOrder.
func gallery(found map[string]bool) []endingTile {
	tiles := make([]endingTile, 0, len(content.EndingOrder))
	for _, class := range content.EndingOrder {
		tiles = append(tiles, endingTile{Class: class, Discovered: found[class]})
	}
	return tiles
}

func (s *Server) home(w http.ResponseWriter, r *http.Request, session string) {
	st := s.store.Get(session)
	if st == nil {
		hist := s.store.History(session)
		found := map[string]bool{}
		for _, run := range hist {
			found[run.Ending] = true
		}
		s.renderPage(w, "title.html", titleView{
			Scene:     s.store.lib.Scenes[startScene],
			Campaigns: len(hist),
			Endings:   gallery(found),
		})
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
	if err == nil && s.store.lib.Scenes[next.SceneID].Ending != "" {
		// The run just ended; record it exactly once (post-ending actions
		// are refused, so this transition cannot repeat).
		if rerr := s.store.CompleteRun(session, next); rerr != nil {
			log.Printf("record run %s: %v", session, rerr)
		}
	}
	s.respondAction(w, r, s.buildView(next, err))
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
// The log renders newest first so the latest action is always on top.
func (s *Server) buildView(st *game.State, err error) view {
	scene := s.store.lib.Scenes[st.SceneID]
	v := view{
		Stats:       st.Stats,
		Scene:       scene,
		Turns:       st.Turns,
		CardsPlayed: st.CardsPlayed,
	}
	hist := s.store.History(st.Session)
	v.Campaigns = len(hist)
	found := map[string]bool{}
	for _, run := range hist {
		found[run.Ending] = true
	}
	v.Endings = gallery(found)
	log := slices.Clone(st.Log)
	if err != nil {
		log = append(log, "Error: "+err.Error())
	}
	slices.Reverse(log)
	v.Log = log
	for i, c := range scene.Choices {
		var reqs []string
		if c.RequiresCard != "" {
			label := s.store.lib.Cards[c.RequiresCard].Name
			if c.ConsumesCard {
				label += " (spent)"
			}
			reqs = append(reqs, label)
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
		c := s.store.lib.Cards[id]
		v.Hand = append(v.Hand, cardView{
			ID:       id,
			Name:     c.Name,
			Text:     c.Text,
			Cost:     c.Cost,
			Playable: st.CanPlay(id, s.store.lib.Cards),
		})
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

// respondAction picks the response shape: htmx requests (HX-Request
// header) get the partial swap set; plain form posts, i.e. browsers
// without JavaScript, get a full page render.
func (s *Server) respondAction(w http.ResponseWriter, r *http.Request, v view) {
	if r.Header.Get("HX-Request") == "true" {
		s.respondPartial(w, v)
		return
	}
	s.renderPage(w, "game.html", v)
}
