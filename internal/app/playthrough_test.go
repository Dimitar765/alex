package app

import (
	"testing"

	assets "goGame"
	"goGame/internal/content"
	"goGame/internal/game"
)

// shippedLib loads the real campaign content.
func shippedLib(t *testing.T) *content.Library {
	t.Helper()
	lib, err := content.Load(assets.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	return lib
}

// TestShippedCampaignChoicesOnlyPlaythrough marches straight through the
// shipped campaign taking the first available choice every turn. Threat
// then advances strictly (no card reductions), so the raid, ambush, and
// forced battle are all guaranteed to fire before an ending arrives.
func TestShippedCampaignChoicesOnlyPlaythrough(t *testing.T) {
	m := NewModel(shippedLib(t), "")
	m.NewGame()

	battleSeen, raidSeen, ambushSeen := false, false, false
	maxThreat := 0
	var ending string
	for turn := 0; turn < 100; turn++ {
		v := m.View()
		if v.Scene.Ending != "" {
			ending = v.Scene.Ending
			break
		}
		if v.SceneID == content.BattleScene {
			battleSeen = true
		}
		if v.Threat > maxThreat {
			maxThreat = v.Threat
		}
		if v.Threat >= game.ThreatRaid {
			raidSeen = true
		}
		if v.Threat >= game.ThreatAmbush {
			ambushSeen = true
		}
		acted := false
		for _, ch := range v.Choices {
			if ch.Available {
				r := m.Choose(ch.Index)
				if r.Err != nil {
					t.Fatalf("turn %d: available choice refused: %v", turn, r.Err)
				}
				acted = true
				break
			}
		}
		if !acted {
			t.Fatalf("turn %d: no available choice at scene %q", turn, v.SceneID)
		}
	}
	if ending == "" {
		t.Fatal("campaign did not reach an ending within 100 turns")
	}
	if !battleSeen {
		t.Fatal("forced battle never reached")
	}
	if !raidSeen || !ambushSeen {
		t.Fatalf("thresholds never crossed: raid=%v ambush=%v maxThreat=%d", raidSeen, ambushSeen, maxThreat)
	}
	if maxThreat != game.ThreatMax {
		t.Fatalf("max threat = %d, want %d", maxThreat, game.ThreatMax)
	}
	if len(m.History()) != 1 {
		t.Fatalf("history = %d runs, want 1", len(m.History()))
	}
}

// TestShippedCampaignMixedPlaythrough plays cards whenever they are
// affordable and otherwise takes choices — closer to a human rhythm. It
// must always reach an ending without engine errors.
func TestShippedCampaignMixedPlaythrough(t *testing.T) {
	m := NewModel(shippedLib(t), "")
	m.NewGame()
	var ending string
	for turn := 0; turn < 120; turn++ {
		v := m.View()
		if v.Scene.Ending != "" {
			ending = v.Scene.Ending
			break
		}
		// One action per turn: prefer playing an affordable card every
		// other turn, otherwise take the first available choice.
		acted := false
		if turn%2 == 0 {
			for _, c := range v.Hand {
				if c.Playable {
					if r := m.PlayCard(c.ID); r.Err == nil {
						acted = true
					}
					break
				}
			}
		}
		if !acted {
			for _, ch := range v.Choices {
				if ch.Available {
					if r := m.Choose(ch.Index); r.Err != nil {
						t.Fatalf("turn %d: %v", turn, r.Err)
					}
					acted = true
					break
				}
			}
		}
		_ = acted
	}
	if ending == "" {
		t.Fatal("mixed playthrough never ended")
	}
}
