// Package content defines the data-driven game content schema (cards and
// scenes) and loads it from JSON with full referential validation.
package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
)

// Effect kinds.
const (
	KindStat       = "stat"
	KindGainCard   = "gain_card"
	KindLoseCard   = "lose_card"
	KindRemoveCard = "remove_card"
	KindGoto       = "goto"
	KindRandom     = "random"
)

// Stat keys usable in stat-effect deltas. The game engine maps each key to
// a player field, so adding one requires a matching case there.
const (
	StatLegacy   = "legacy"
	StatArmy     = "army"
	StatTreasury = "treasury"
)

// statKeys is the closed set accepted in Effect.Delta and Choice.RequiresStat.
var statKeys = map[string]bool{
	StatLegacy:   true,
	StatArmy:     true,
	StatTreasury: true,
}

// Endings is the closed set of ending classifications for terminal scenes.
// The values drive the UI badge and its styling.
var Endings = map[string]bool{
	"triumph": true,
	"legacy":  true,
	"settle":  true,
	"defeat":  true,
	"death":   true,
}

// EndingOrder is the display order of ending classifications for galleries.
var EndingOrder = []string{"triumph", "legacy", "settle", "defeat", "death"}

// Outcome is one weighted branch of a random effect. Weight is relative
// likelihood and must be at least 1.
type Outcome struct {
	Weight  int      `json:"weight"`
	Effects []Effect `json:"effects"`
}

// Effect is one game-rule step. Kind selects which other field is used:
// stat -> Delta, gain_card/lose_card/remove_card -> CardID, goto -> Next,
// random -> Outcomes.
type Effect struct {
	Kind     string         `json:"kind"`
	CardID   string         `json:"cardId,omitempty"`
	Delta    map[string]int `json:"delta,omitempty"`
	Next     string         `json:"next,omitempty"`
	Outcomes []Outcome      `json:"outcomes,omitempty"`
}

// Card is a playable card. Cost is the treasury paid to play it (0 is
// free). Effects apply in declared order.
type Card struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Text    string   `json:"text"`
	Cost    int      `json:"cost,omitempty"`
	Effects []Effect `json:"effects"`
}

// Choice is one option on a scene. RequiresCard and RequiresStat are
// minimum requirements: the card must be in hand and every stat must be at
// or above its threshold. ConsumesCard spends the required card to the
// discard pile when the choice is taken.
type Choice struct {
	Text         string         `json:"text"`
	RequiresCard string         `json:"requiresCard,omitempty"`
	ConsumesCard bool           `json:"consumesCard,omitempty"`
	RequiresStat map[string]int `json:"requiresStat,omitempty"`
	Effects      []Effect       `json:"effects"`
}

// Scene is one narrative location with its choices. A scene with a
// non-empty Ending is terminal: it renders the run summary instead of
// choices and must declare no choices.
type Scene struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Ending  string   `json:"ending,omitempty"`
	Choices []Choice `json:"choices"`
}

// Library is validated content: cards and scenes keyed by ID.
type Library struct {
	Cards  map[string]Card
	Scenes map[string]Scene
}

// CardList returns all cards sorted by ID for deterministic deck setup.
func (l *Library) CardList() []Card {
	ids := slices.Sorted(maps.Keys(l.Cards))
	out := make([]Card, 0, len(ids))
	for _, id := range ids {
		out = append(out, l.Cards[id])
	}
	return out
}

// Load decodes cards.json and scenes.json from fsys and validates
// referential integrity. It returns one error listing every violation.
func Load(fsys fs.FS) (*Library, error) {
	lib := &Library{Cards: map[string]Card{}, Scenes: map[string]Scene{}}
	var probs []string
	problemf := func(format string, args ...any) { probs = append(probs, fmt.Sprintf(format, args...)) }

	var cards []Card
	if err := decode(fsys, "cards.json", &cards); err != nil {
		return nil, err
	}
	for _, c := range cards {
		if c.ID == "" {
			problemf("card with empty id")
			continue
		}
		if _, dup := lib.Cards[c.ID]; dup {
			problemf("duplicate card id %q", c.ID)
			continue
		}
		if c.Cost < 0 {
			problemf("card %q: negative cost %d", c.ID, c.Cost)
		}
		lib.Cards[c.ID] = c
	}

	var scenes []Scene
	if err := decode(fsys, "scenes.json", &scenes); err != nil {
		return nil, err
	}
	for _, sc := range scenes {
		if sc.ID == "" {
			problemf("scene with empty id")
			continue
		}
		if _, dup := lib.Scenes[sc.ID]; dup {
			problemf("duplicate scene id %q", sc.ID)
			continue
		}
		lib.Scenes[sc.ID] = sc
	}

	for _, c := range cards {
		for i, e := range c.Effects {
			if err := validateEffect(e, lib); err != nil {
				problemf("card %q effect %d: %v", c.ID, i, err)
			}
		}
	}
	for _, sc := range scenes {
		if sc.Ending != "" {
			if !Endings[sc.Ending] {
				problemf("scene %q: unknown ending %q", sc.ID, sc.Ending)
			}
			if len(sc.Choices) != 0 {
				problemf("ending scene %q must have no choices", sc.ID)
			}
		} else if len(sc.Choices) == 0 {
			problemf("scene %q has no choices (dead end)", sc.ID)
		}
		for i, ch := range sc.Choices {
			if ch.RequiresCard != "" {
				if _, ok := lib.Cards[ch.RequiresCard]; !ok {
					problemf("scene %q choice %d: requires unknown card %q", sc.ID, i, ch.RequiresCard)
				}
			} else if ch.ConsumesCard {
				problemf("scene %q choice %d: consumesCard requires a requiresCard", sc.ID, i)
			}
			for _, k := range slices.Sorted(maps.Keys(ch.RequiresStat)) {
				if !statKeys[k] {
					problemf("scene %q choice %d: requiresStat with unknown stat %q", sc.ID, i, k)
				}
			}
			for j, e := range ch.Effects {
				if err := validateEffect(e, lib); err != nil {
					problemf("scene %q choice %d effect %d: %v", sc.ID, i, j, err)
				}
			}
		}
	}
	if _, ok := lib.Scenes["title"]; !ok {
		problemf(`no "title" scene`)
	} else {
		for id := range unreachable(lib, cards) {
			problemf("scene %q is unreachable from title", id)
		}
	}

	if len(probs) > 0 {
		return nil, fmt.Errorf("invalid content:\n  - %s", strings.Join(probs, "\n  - "))
	}
	return lib, nil
}

func validateEffect(e Effect, lib *Library) error {
	switch e.Kind {
	case KindStat:
		if len(e.Delta) == 0 {
			return fmt.Errorf("stat effect with empty delta")
		}
		for _, k := range slices.Sorted(maps.Keys(e.Delta)) {
			if !statKeys[k] {
				return fmt.Errorf("stat effect with unknown stat %q", k)
			}
		}
	case KindGainCard, KindLoseCard, KindRemoveCard:
		if _, ok := lib.Cards[e.CardID]; !ok {
			return fmt.Errorf("%s references unknown card %q", e.Kind, e.CardID)
		}
	case KindGoto:
		if _, ok := lib.Scenes[e.Next]; !ok {
			return fmt.Errorf("goto references unknown scene %q", e.Next)
		}
	case KindRandom:
		if len(e.Outcomes) == 0 {
			return fmt.Errorf("random effect with no outcomes")
		}
		var errs []error
		for i, o := range e.Outcomes {
			if o.Weight < 1 {
				errs = append(errs, fmt.Errorf("random outcome %d: weight must be >= 1", i))
				continue
			}
			for j, sub := range o.Effects {
				if err := validateEffect(sub, lib); err != nil {
					errs = append(errs, fmt.Errorf("random outcome %d effect %d: %w", i, j, err))
				}
			}
		}
		return errors.Join(errs...)
	default:
		return fmt.Errorf("unknown kind %q", e.Kind)
	}
	return nil
}

// gotoTargets adds every scene id referenced by effects, recursing through
// random outcomes, to out.
func gotoTargets(effects []Effect, out map[string]bool) {
	for _, e := range effects {
		switch e.Kind {
		case KindGoto:
			out[e.Next] = true
		case KindRandom:
			for _, o := range e.Outcomes {
				gotoTargets(o.Effects, out)
			}
		}
	}
}

// unreachable returns the set of scenes that cannot be reached from title.
// Edges are scene choices; card effects may be played from any scene, so
// their goto targets count as edges from everywhere.
func unreachable(lib *Library, cards []Card) map[string]bool {
	cardEdges := map[string]bool{}
	for _, c := range cards {
		gotoTargets(c.Effects, cardEdges)
	}
	reached := map[string]bool{}
	queue := []string{"title"}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if reached[id] {
			continue
		}
		reached[id] = true
		next := map[string]bool{}
		for _, ch := range lib.Scenes[id].Choices {
			gotoTargets(ch.Effects, next)
		}
		maps.Copy(next, cardEdges)
		for t := range next {
			if !reached[t] {
				queue = append(queue, t)
			}
		}
	}
	unreached := map[string]bool{}
	for id := range lib.Scenes {
		if !reached[id] {
			unreached[id] = true
		}
	}
	return unreached
}

func decode[T any](fsys fs.FS, name string, v *T) error {
	f, err := fsys.Open(name)
	if err != nil {
		return fmt.Errorf("content: %w", err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
