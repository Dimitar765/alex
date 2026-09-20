package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

const happyCards = `[
  {"id":"phalanx","name":"Phalanx","text":"A wall.","effects":[{"kind":"stat","delta":{"army":2}}]},
  {"id":"decreed_alliance","name":"Decreed Alliance","text":"Paper walls.","effects":[{"kind":"gain_card","cardId":"phalanx"}]}
]`

const happyScenes = `[
  {"id":"title","text":"You stand.","choices":[{"text":"March","effects":[{"kind":"goto","next":"field"}]}]},
  {"id":"field","text":"A field.","choices":[{"text":"Return","effects":[{"kind":"stat","delta":{"treasury":1}},{"kind":"goto","next":"title"}]}]}
]`

func fsFrom(t *testing.T, cards, scenes string) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"cards.json":  &fstest.MapFile{Data: []byte(cards)},
		"scenes.json": &fstest.MapFile{Data: []byte(scenes)},
	}
}

func TestLoadValid(t *testing.T) {
	lib, err := Load(fsFrom(t, happyCards, happyScenes))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, ok := lib.Cards["phalanx"]; !ok {
		t.Fatal("phalanx card missing")
	}
	if _, ok := lib.Scenes["title"]; !ok {
		t.Fatal("title scene missing")
	}
	list := lib.CardList()
	if len(list) != 2 || list[0].ID != "decreed_alliance" || list[1].ID != "phalanx" {
		t.Fatalf("CardList() = %v, want sorted by ID", list)
	}
}

func TestLoadDanglingNext(t *testing.T) {
	scenes := `[
      {"id":"title","text":"You stand.","choices":[{"text":"March","effects":[{"kind":"goto","next":"nowhere"}]}]}
    ]`
	_, err := Load(fsFrom(t, happyCards, scenes))
	if err == nil {
		t.Fatal("Load() must fail on dangling goto target")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Fatalf("error must name the dangling ID, got: %v", err)
	}
}

func TestLoadAggregatesViolations(t *testing.T) {
	cards := `[{"id":"x","name":"X","text":"","effects":[{"kind":"gain_card","cardId":"ghost"}]}]`
	scenes := `[
      {"id":"title","text":"","choices":[{"text":"A","requiresCard":"ghost2","effects":[]},{"text":"B","effects":[{"kind":"goto","next":"void"}]}]},
      {"id":"title","text":"","choices":[]}
    ]`
	_, err := Load(fsFrom(t, cards, scenes))
	if err == nil {
		t.Fatal("Load() must fail")
	}
	for _, want := range []string{"ghost", "ghost2", "void", "duplicate scene id"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q, got:\n%v", want, err)
		}
	}
}

func TestLoadUnknownStatKeyRejected(t *testing.T) {
	cards := `[{"id":"x","name":"X","text":"","effects":[{"kind":"stat","delta":{"legassy":1}}]}]`
	_, err := Load(fsFrom(t, cards, happyScenes))
	if err == nil {
		t.Fatal("Load() must reject unknown stat keys at load time")
	}
	if !strings.Contains(err.Error(), "legassy") {
		t.Fatalf("error must name the bad stat key, got: %v", err)
	}
}

func TestLoadRandomEffectValidation(t *testing.T) {
	cards := `[{"id":"x","name":"X","text":"","effects":[
      {"kind":"random","outcomes":[
        {"weight":0,"effects":[{"kind":"stat","delta":{"army":1}}]},
        {"weight":1,"effects":[{"kind":"gain_card","cardId":"ghost"}]}
      ]}
    ]}]`
	_, err := Load(fsFrom(t, cards, happyScenes))
	if err == nil {
		t.Fatal("Load() must reject bad random outcomes")
	}
	for _, want := range []string{"weight must be >= 1", "outcome 1 effect 0", "ghost"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q, got:\n%v", want, err)
		}
	}
}

func TestLoadRequiresStatValidation(t *testing.T) {
	scenes := `[
      {"id":"title","text":"","choices":[{"text":"A","requiresStat":{"armee":2},"effects":[]}]},
      {"id":"field","text":"","choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]}
    ]`
	if _, err := Load(fsFrom(t, happyCards, scenes)); err == nil {
		t.Fatal("Load() must reject unknown requiresStat keys")
	} else if !strings.Contains(err.Error(), "armee") {
		t.Fatalf("error must name the bad stat, got: %v", err)
	}
}

func TestLoadEndingRules(t *testing.T) {
	cards := `[{"id":"x","name":"X","text":"","effects":[]}]`
	scenes := `[
      {"id":"title","text":"","choices":[{"text":"A","effects":[{"kind":"goto","next":"end1"}]}]},
      {"id":"end1","text":"","ending":"triumph","choices":[{"text":"nope","effects":[]}]},
      {"id":"end2","text":"","ending":"apolcalypse"}
    ]`
	_, err := Load(fsFrom(t, cards, scenes))
	if err == nil {
		t.Fatal("Load() must reject malformed ending scenes")
	}
	for _, want := range []string{"must have no choices", "unknown ending"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q, got:\n%v", want, err)
		}
	}
}

func TestLoadUnreachableSceneRejected(t *testing.T) {
	scenes := `[
      {"id":"title","text":"","choices":[{"text":"A","effects":[{"kind":"goto","next":"field"}]}]},
      {"id":"field","text":"","choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]},
      {"id":"island","text":"","choices":[{"text":"C","effects":[{"kind":"goto","next":"title"}]}]}
    ]`
	_, err := Load(fsFrom(t, happyCards, scenes))
	if err == nil {
		t.Fatal("Load() must reject unreachable scenes")
	}
	if !strings.Contains(err.Error(), `scene "island" is unreachable`) {
		t.Fatalf("error must name the unreachable scene, got: %v", err)
	}
}

func TestLoadRandomOutcomesMakeScenesReachable(t *testing.T) {
	scenes := `[
      {"id":"title","text":"","choices":[{"text":"A","effects":[
        {"kind":"random","outcomes":[
          {"weight":1,"effects":[{"kind":"goto","next":"field"}]},
          {"weight":1,"effects":[{"kind":"goto","next":"end1"}]}
        ]}
      ]}]},
      {"id":"field","text":"","choices":[{"text":"B","effects":[{"kind":"goto","next":"title"}]}]},
      {"id":"end1","text":"","ending":"triumph"}
    ]`
	if _, err := Load(fsFrom(t, happyCards, scenes)); err != nil {
		t.Fatalf("gotos inside random outcomes must count as edges: %v", err)
	}
}

func TestLoadUnknownFieldsRejected(t *testing.T) {
	cards := `[{"id":"x","name":"X","text":"","effects":[],"bogus":1}]`
	if _, err := Load(fsFrom(t, cards, happyScenes)); err == nil {
		t.Fatal("Load() must reject unknown JSON fields")
	}
}
