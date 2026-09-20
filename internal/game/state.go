// Package game implements the rules engine: player state, scene choices,
// and card play. It is the only place game rules live.
package game

import (
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"

	"goGame/internal/content"
)

// HandSize is the number of cards the hand refills to.
const HandSize = 4

// MaxLogLen bounds the human-readable event log.
const MaxLogLen = 50

// Stats are the tracked resources. They may go negative by design.
type Stats struct {
	Legacy   int
	Army     int
	Treasury int
}

// State is one player's full game state. It is plain data so it serializes
// to the save directory as JSON.
type State struct {
	Session string
	SceneID string
	Stats   Stats
	Hand    []string
	Deck    []string // draw pile; the last element is the top
	Discard []string
	Log     []string
	Granted []string // scenes whose card pools have been granted

	Turns       int // successful actions taken (choices and card plays)
	CardsPlayed int // card plays among those turns

	rng *rand.Rand // optional deterministic source; nil uses the global
}

// NewState builds a fresh state from the library's starting cards: a
// shuffled opening deck and a hand of HandSize.
func NewState(lib *content.Library, startScene string) *State {
	return NewStateWithRNG(lib, startScene, nil)
}

// NewStateWithRNG is NewState with an explicit random source, used by the
// simulator for reproducible runs. A nil rng falls back to the global one.
func NewStateWithRNG(lib *content.Library, startScene string, rng *rand.Rand) *State {
	s := &State{SceneID: startScene, rng: rng}
	for _, c := range lib.StartingCards() {
		s.Deck = append(s.Deck, c.ID)
	}
	s.shuffle(len(s.Deck), func(i, j int) { s.Deck[i], s.Deck[j] = s.Deck[j], s.Deck[i] })
	s.drawUp()
	return s
}

// shuffle randomizes n elements via the state's random source.
func (s *State) shuffle(n int, swap func(i, j int)) {
	if s.rng != nil {
		s.rng.Shuffle(n, swap)
		return
	}
	rand.Shuffle(n, swap)
}

// intN returns a random value in [0,n) from the state's random source.
func (s *State) intN(n int) int {
	if s.rng != nil {
		return s.rng.IntN(n)
	}
	return rand.IntN(n)
}

// Clone returns a deep copy of the state. Mutating the clone never affects
// the original, so callers can apply an action speculatively and commit the
// result only when it succeeds.
func (s *State) Clone() *State {
	c := *s
	c.Hand = slices.Clone(s.Hand)
	c.Deck = slices.Clone(s.Deck)
	c.Discard = slices.Clone(s.Discard)
	c.Log = slices.Clone(s.Log)
	c.Granted = slices.Clone(s.Granted)
	return &c
}

// Choose applies a scene choice. It fails if the choice's card or stat
// requirements are not met. A consuming choice spends its required card to
// the discard pile. Effects apply in declared order; arriving at a new
// scene grants its card pool once; the hand then refills.
func (s *State) Choose(choice content.Choice, lib *content.Library) error {
	if err := s.requirementError(choice, lib.Cards); err != nil {
		return err
	}
	parts := []string{choice.Text}
	if choice.ConsumesCard {
		takeCard(choice.RequiresCard, &s.Hand)
		s.Discard = append(s.Discard, choice.RequiresCard)
		parts = append(parts, "spent "+displayName(choice.RequiresCard, lib.Cards))
	}
	before := s.SceneID
	if err := s.applyEffects(choice.Effects, lib, &parts); err != nil {
		return err
	}
	if s.SceneID != before {
		s.arrive(lib, &parts)
	}
	s.drawUp()
	s.Turns++
	s.appendLog(joinParts(parts))
	return nil
}

// PlayCard removes a card from the hand to the discard pile, pays its cost,
// applies its effects in order, and refills the hand. It fails if the card
// is not in hand or the treasury cannot cover the cost.
func (s *State) PlayCard(id string, lib *content.Library) error {
	i := slices.Index(s.Hand, id)
	if i < 0 {
		return fmt.Errorf("%s is not in your hand", displayName(id, lib.Cards))
	}
	card := lib.Cards[id]
	if s.Stats.Treasury < card.Cost {
		return fmt.Errorf("you cannot afford %s (costs %d treasury)", displayName(id, lib.Cards), card.Cost)
	}
	s.Hand = slices.Delete(s.Hand, i, i+1)
	s.Discard = append(s.Discard, id)
	parts := []string{"Played " + displayName(id, lib.Cards)}
	if card.Cost > 0 {
		s.Stats.Treasury -= card.Cost
		parts = append(parts, fmt.Sprintf("Treasury -%d", card.Cost))
	}
	before := s.SceneID
	if err := s.applyEffects(card.Effects, lib, &parts); err != nil {
		return err
	}
	if s.SceneID != before {
		s.arrive(lib, &parts)
	}
	s.drawUp()
	s.Turns++
	s.CardsPlayed++
	s.appendLog(joinParts(parts))
	return nil
}

// CanPlay reports whether the card is in hand and affordable right now.
func (s *State) CanPlay(id string, cards map[string]content.Card) bool {
	if !slices.Contains(s.Hand, id) {
		return false
	}
	return s.Stats.Treasury >= cards[id].Cost
}

// Shuffle spends the turn recycling the discard pile into the deck. It
// fails when there is nothing to shuffle.
func (s *State) Shuffle() error {
	if len(s.Discard) == 0 {
		return errors.New("nothing to shuffle — the discard pile is empty")
	}
	s.Deck = append(s.Deck, s.Discard...)
	s.Discard = nil
	s.shuffle(len(s.Deck), func(i, j int) { s.Deck[i], s.Deck[j] = s.Deck[j], s.Deck[i] })
	s.Turns++
	s.appendLog("Shuffled the discard pile into the deck")
	return nil
}

// arrive grants the entered scene's regional card pool on first entry,
// stacking the cards on the deck so they are drawn first.
func (s *State) arrive(lib *content.Library, parts *[]string) {
	sc := lib.Scenes[s.SceneID]
	if len(sc.Cards) == 0 || slices.Contains(s.Granted, sc.ID) {
		return
	}
	s.Granted = append(s.Granted, sc.ID)
	for _, id := range sc.Cards {
		s.Deck = append(s.Deck, id)
		*parts = append(*parts, "gained "+displayName(id, lib.Cards))
	}
}

// CanChoose reports whether the current hand and stats meet every
// requirement on the choice.
func (s *State) CanChoose(c content.Choice) bool {
	return s.requirementError(c, nil) == nil
}

// requirementError returns the first unmet requirement as a player-facing
// error, or nil when the choice is available.
func (s *State) requirementError(c content.Choice, cards map[string]content.Card) error {
	if c.RequiresCard != "" && !slices.Contains(s.Hand, c.RequiresCard) {
		return fmt.Errorf("that path requires %s", displayName(c.RequiresCard, cards))
	}
	for _, k := range slices.Sorted(maps.Keys(c.RequiresStat)) {
		if have := s.Stat(k); have < c.RequiresStat[k] {
			return fmt.Errorf("that path requires %s %d (you have %d)", StatLabel(k), c.RequiresStat[k], have)
		}
	}
	return nil
}

// Stat returns the current value of a stat key, or 0 for unknown keys.
func (s *State) Stat(k string) int {
	switch k {
	case content.StatLegacy:
		return s.Stats.Legacy
	case content.StatArmy:
		return s.Stats.Army
	case content.StatTreasury:
		return s.Stats.Treasury
	}
	return 0
}

func (s *State) applyEffects(effects []content.Effect, lib *content.Library, parts *[]string) error {
	for _, e := range effects {
		if err := s.apply(e, lib, parts); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) apply(e content.Effect, lib *content.Library, parts *[]string) error {
	switch e.Kind {
	case content.KindStat:
		for _, k := range slices.Sorted(maps.Keys(e.Delta)) {
			v := e.Delta[k]
			switch k {
			case content.StatLegacy:
				s.Stats.Legacy += v
			case content.StatArmy:
				s.Stats.Army += v
			case content.StatTreasury:
				s.Stats.Treasury += v
			default:
				return fmt.Errorf("unknown stat %q", k)
			}
			*parts = append(*parts, fmt.Sprintf("%s %+d", StatLabel(k), v))
		}
	case content.KindGainCard:
		// Deck top = last element, so an appended card is drawn first.
		s.Deck = append(s.Deck, e.CardID)
		*parts = append(*parts, "gained "+displayName(e.CardID, lib.Cards))
	case content.KindLoseCard:
		// Lost for now: the card surfaces again when the discard cycles.
		if takeCard(e.CardID, &s.Hand, &s.Deck) {
			s.Discard = append(s.Discard, e.CardID)
			*parts = append(*parts, "lost "+displayName(e.CardID, lib.Cards))
		}
	case content.KindRemoveCard:
		if takeCard(e.CardID, &s.Hand, &s.Deck, &s.Discard) {
			*parts = append(*parts, displayName(e.CardID, lib.Cards)+" is gone for good")
		}
	case content.KindGoto:
		s.SceneID = e.Next
	case content.KindRandom:
		total := 0
		for _, o := range e.Outcomes {
			total += o.Weight
		}
		pick := s.intN(total)
		for _, o := range e.Outcomes {
			if pick < o.Weight {
				return s.applyEffects(o.Effects, lib, parts)
			}
			pick -= o.Weight
		}
	default:
		return fmt.Errorf("unknown effect kind %q", e.Kind)
	}
	return nil
}

// takeCard removes one copy of id from the first pile that holds it and
// reports whether a copy was found.
func takeCard(id string, piles ...*[]string) bool {
	for _, p := range piles {
		if i := slices.Index(*p, id); i >= 0 {
			*p = slices.Delete(*p, i, i+1)
			return true
		}
	}
	return false
}

// drawUp refills the hand to HandSize, reshuffling the discard pile into the
// deck when the deck runs dry.
func (s *State) drawUp() {
	for len(s.Hand) < HandSize {
		if len(s.Deck) == 0 {
			if len(s.Discard) == 0 {
				return
			}
			s.Deck = s.Discard
			s.Discard = nil
			s.shuffle(len(s.Deck), func(i, j int) { s.Deck[i], s.Deck[j] = s.Deck[j], s.Deck[i] })
		}
		s.Hand = append(s.Hand, s.Deck[len(s.Deck)-1])
		s.Deck = s.Deck[:len(s.Deck)-1]
	}
}

func (s *State) appendLog(line string) {
	s.Log = append(s.Log, line)
	if n := len(s.Log) - MaxLogLen; n > 0 {
		s.Log = s.Log[n:]
	}
}

func joinParts(parts []string) string {
	line := parts[0] + "."
	if len(parts) > 1 {
		line += " " + strings.Join(parts[1:], ", ") + "."
	}
	return line
}

// StatLabel returns the display name for a stat key.
func StatLabel(k string) string {
	switch k {
	case content.StatLegacy:
		return "Legacy"
	case content.StatArmy:
		return "Army"
	case content.StatTreasury:
		return "Treasury"
	}
	return k
}

func displayName(id string, cards map[string]content.Card) string {
	if c, ok := cards[id]; ok && c.Name != "" {
		return c.Name
	}
	return id
}
