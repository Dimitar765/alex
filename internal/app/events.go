package app

// EffectKind classifies one visualizable consequence of an action. The
// view-model derives effects by diffing the state before and after a
// committed action, so the UI never needs game-rule knowledge.
type EffectKind int

const (
	// EffectStat: a stat moved. Stat names the key, Delta the change.
	EffectStat EffectKind = iota
	// EffectCardLeft: a card left the hand (played or spent).
	EffectCardLeft
	// EffectCardGained: a card entered the run's pool (scene arrival
	// or a gain effect).
	EffectCardGained
	// EffectCardGone: a card left the pool for good.
	EffectCardGone
	// EffectSceneChanged: the narrative moved. From/To hold scene IDs.
	EffectSceneChanged
	// EffectEnding: the run reached a terminal scene. Text holds the
	// ending classification (e.g. "triumph").
	EffectEnding
	// EffectFate: the action resolved a random effect.
	EffectFate
	// EffectThreat: the threat track moved. Stat is "threat", Delta the
	// change.
	EffectThreat
)

// Effect is one consequence of a committed action. Which fields are
// meaningful depends on Kind.
type Effect struct {
	Kind   EffectKind
	Stat   string
	Delta  int
	CardID string
	From   string
	To     string
	Text   string
}
