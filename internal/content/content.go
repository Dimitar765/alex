// Package content defines the data-driven game content schema (cards and
// scenes) and loads it from JSON with full referential validation.
package content

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
)

// Effect kinds.
const (
	KindStat     = "stat"
	KindGainCard = "gain_card"
	KindGoto     = "goto"
)

// Stat keys usable in stat-effect deltas. The game engine maps each key to
// a player field, so adding one requires a matching case there.
const (
	StatLegacy   = "legacy"
	StatArmy     = "army"
	StatTreasury = "treasury"
)

// statKeys is the closed set accepted in Effect.Delta.
var statKeys = map[string]bool{
	StatLegacy:   true,
	StatArmy:     true,
	StatTreasury: true,
}

// Effect is one game-rule step. Kind selects which other field is used:
// stat -> Delta, gain_card -> CardID, goto -> Next.
type Effect struct {
	Kind   string         `json:"kind"`
	CardID string         `json:"cardId,omitempty"`
	Delta  map[string]int `json:"delta,omitempty"`
	Next   string         `json:"next,omitempty"`
}

// Card is a playable card. Effects apply in declared order.
type Card struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Text    string   `json:"text"`
	Effects []Effect `json:"effects"`
}

// Choice is one option on a scene.
type Choice struct {
	Text         string   `json:"text"`
	RequiresCard string   `json:"requiresCard,omitempty"`
	Effects      []Effect `json:"effects"`
}

// Scene is one narrative location with its choices.
type Scene struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
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
		if len(sc.Choices) == 0 {
			problemf("scene %q has no choices (dead end)", sc.ID)
		}
		for i, ch := range sc.Choices {
			if ch.RequiresCard != "" {
				if _, ok := lib.Cards[ch.RequiresCard]; !ok {
					problemf("scene %q choice %d: requires unknown card %q", sc.ID, i, ch.RequiresCard)
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
	case KindGainCard:
		if _, ok := lib.Cards[e.CardID]; !ok {
			return fmt.Errorf("gain_card references unknown card %q", e.CardID)
		}
	case KindGoto:
		if _, ok := lib.Scenes[e.Next]; !ok {
			return fmt.Errorf("goto references unknown scene %q", e.Next)
		}
	default:
		return fmt.Errorf("unknown kind %q", e.Kind)
	}
	return nil
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
