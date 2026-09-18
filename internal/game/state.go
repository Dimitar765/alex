// Package game implements the rules engine: player state, scene choices,
// and card play. It is the only place game rules live.
package game

import (
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
}

// NewState builds a fresh state: shuffled deck, opening hand of HandSize.
func NewState(cards []content.Card, startScene string) *State {
	s := &State{SceneID: startScene}
	for _, c := range cards {
		s.Deck = append(s.Deck, c.ID)
	}
	rand.Shuffle(len(s.Deck), func(i, j int) { s.Deck[i], s.Deck[j] = s.Deck[j], s.Deck[i] })
	s.drawUp()
	return s
}

// Choose applies a scene choice. It fails if the choice requires a card that
// is not in hand. Effects apply in declared order; the hand then refills.
func (s *State) Choose(choice content.Choice, cards map[string]content.Card) error {
	if choice.RequiresCard != "" && !slices.Contains(s.Hand, choice.RequiresCard) {
		return fmt.Errorf("that path requires %s", displayName(choice.RequiresCard, cards))
	}
	parts := []string{choice.Text}
	for _, e := range choice.Effects {
		if err := s.apply(e, cards, &parts); err != nil {
			return err
		}
	}
	s.drawUp()
	s.appendLog(joinParts(parts))
	return nil
}

// PlayCard removes a card from the hand to the discard pile, applies its
// effects in order, and refills the hand. It fails if the card is not in hand.
func (s *State) PlayCard(id string, cards map[string]content.Card) error {
	i := slices.Index(s.Hand, id)
	if i < 0 {
		return fmt.Errorf("%s is not in your hand", displayName(id, cards))
	}
	card := cards[id]
	s.Hand = slices.Delete(s.Hand, i, i+1)
	s.Discard = append(s.Discard, id)
	parts := []string{"Played " + displayName(id, cards)}
	for _, e := range card.Effects {
		if err := s.apply(e, cards, &parts); err != nil {
			return err
		}
	}
	s.drawUp()
	s.appendLog(joinParts(parts))
	return nil
}

func (s *State) apply(e content.Effect, cards map[string]content.Card, parts *[]string) error {
	switch e.Kind {
	case content.KindStat:
		for _, k := range slices.Sorted(maps.Keys(e.Delta)) {
			v := e.Delta[k]
			switch k {
			case "legacy":
				s.Stats.Legacy += v
			case "army":
				s.Stats.Army += v
			case "treasury":
				s.Stats.Treasury += v
			default:
				return fmt.Errorf("unknown stat %q", k)
			}
			*parts = append(*parts, fmt.Sprintf("%s %+d", statLabel(k), v))
		}
	case content.KindGainCard:
		// Deck top = last element, so an appended card is drawn first.
		s.Deck = append(s.Deck, e.CardID)
		*parts = append(*parts, "gained "+displayName(e.CardID, cards))
	case content.KindGoto:
		s.SceneID = e.Next
	default:
		return fmt.Errorf("unknown effect kind %q", e.Kind)
	}
	return nil
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
			rand.Shuffle(len(s.Deck), func(i, j int) { s.Deck[i], s.Deck[j] = s.Deck[j], s.Deck[i] })
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

func statLabel(k string) string {
	switch k {
	case "legacy":
		return "Legacy"
	case "army":
		return "Army"
	case "treasury":
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
