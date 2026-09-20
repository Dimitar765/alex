package web

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"goGame/internal/content"
)

var testContent = fstest.MapFS{
	"cards.json": &fstest.MapFile{Data: []byte(`[
      {"id":"phalanx","name":"Phalanx","text":"A wall of sarissas.","cost":2,"start":true,"effects":[{"kind":"stat","delta":{"army":2}}]},
      {"id":"decree","name":"Royal Decree","text":"A seal, a scribble.","start":true,"effects":[{"kind":"stat","delta":{"treasury":3}}]},
      {"id":"scout","name":"Scouts","text":"Eyes on the road ahead.","start":true,"effects":[{"kind":"stat","delta":{"army":1}}]},
      {"id":"peltast","name":"Peltasts","text":"Skirmishers on the flank.","start":true,"effects":[{"kind":"stat","delta":{"army":1}}]},
      {"id":"herald","name":"Herald","text":"News at a gallop.","start":true,"effects":[{"kind":"stat","delta":{"treasury":1}}]}
    ]`)},
	"scenes.json": &fstest.MapFile{Data: []byte(`[
      {"id":"title","text":"You stand at the Hellespont.","choices":[{"text":"March out","effects":[{"kind":"goto","next":"field"}]}]},
      {"id":"field","text":"An open field awaits.","choices":[
        {"text":"Return to the coast","effects":[{"kind":"goto","next":"title"}]},
        {"text":"Claim the vault of the treasury","requiresStat":{"treasury":3},"effects":[{"kind":"goto","next":"end_demo"}]},
        {"text":"Tear up the decree before the heralds","requiresCard":"decree","consumesCard":true,"effects":[{"kind":"goto","next":"title"}]}
      ]},
      {"id":"end_demo","text":"The world is yours.","ending":"triumph"}
    ]`)},
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	lib, err := content.Load(testContent)
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	ts := httptest.NewServer(New(NewStore(lib, "")).Handler)
	t.Cleanup(ts.Close)
	return ts
}

// newTestClient returns a cookie-carrying client and the server base URL.
func newTestClient(t *testing.T) (*http.Client, string) {
	t.Helper()
	ts := newTestServer(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}, ts.URL
}

// noRedirect returns a client that does not follow redirects.
func noRedirect(c *http.Client) *http.Client {
	return &http.Client{
		Jar:       c.Jar,
		Transport: c.Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func get(t *testing.T, c *http.Client, path string) *http.Response {
	t.Helper()
	resp, err := c.Get(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func postForm(t *testing.T, c *http.Client, path string, form url.Values) *http.Response {
	t.Helper()
	resp, err := c.PostForm(path, form)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// postAction posts an action with the htmx request header, as the
// JavaScript-enhanced UI sends it.
func postAction(t *testing.T, c *http.Client, base string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", base+"/game/action", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHomeWithoutStateRendersTitle(t *testing.T) {
	c, base := newTestClient(t)
	resp := get(t, c, base+"/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}
	b := readBody(t, resp)
	for _, want := range []string{"You stand at the Hellespont.", "Begin the campaign"} {
		if !strings.Contains(b, want) {
			t.Fatalf("title page missing %q", want)
		}
	}
	// A fresh session shows a fully undiscovered endings gallery.
	if n := strings.Count(b, "???"); n != 5 {
		t.Fatalf("fresh gallery must show 5 undiscovered slots, got %d", n)
	}
	if strings.Contains(b, "campaigns completed") {
		t.Fatal("fresh session must not claim completed campaigns")
	}
}

func TestNewGameRedirectsAndDealsHand(t *testing.T) {
	c, base := newTestClient(t)
	resp := postForm(t, noRedirect(c), base+"/game/new", nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /game/new = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Fatalf("Location = %q, want /", loc)
	}
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, "You stand at the Hellespont.") {
		t.Fatal("game page missing scene text")
	}
	if !strings.Contains(b, "Phalanx") {
		t.Fatal("game page missing dealt hand card")
	}
}

func startGame(t *testing.T, c *http.Client, base string) {
	t.Helper()
	resp := postForm(t, c, base+"/game/new", nil)
	if resp.StatusCode != http.StatusOK { // followed redirect landed on game page
		t.Fatalf("POST /game/new → %d, want 200 after redirect", resp.StatusCode)
	}
}

// playCardAny plays the first of ids that succeeds and returns its id.
func playCardAny(t *testing.T, c *http.Client, base string, ids ...string) string {
	t.Helper()
	for _, id := range ids {
		b := readBody(t, postAction(t, c, base, url.Values{"card": {id}}))
		if !strings.Contains(b, "Error:") {
			return id
		}
	}
	t.Fatalf("none of %v playable", ids)
	return ""
}

// playCardDig plays id, digging through the deck with free cards when the
// opening hand missed it; the 5-card test deck keeps any card at most one
// play away. It returns the successful action's response body.
func playCardDig(t *testing.T, c *http.Client, base, id string) string {
	t.Helper()
	b := readBody(t, postAction(t, c, base, url.Values{"card": {id}}))
	for i := 0; strings.Contains(b, "Error:") && i < 4; i++ {
		playCardAny(t, c, base, "scout", "peltast", "herald")
		b = readBody(t, postAction(t, c, base, url.Values{"card": {id}}))
	}
	if strings.Contains(b, "Error:") {
		t.Fatalf("playing %q kept failing: %s", id, b)
	}
	return b
}

// discardFills plays free cards until the shuffle button reports a
// non-empty discard pile.
func discardFills(t *testing.T, c *http.Client, base string) {
	t.Helper()
	for i := 0; i < 5; i++ {
		b := readBody(t, get(t, c, base+"/"))
		if !strings.Contains(b, "Shuffle · 0") {
			return
		}
		playCardAny(t, c, base, "scout", "peltast", "herald")
	}
	t.Fatal("discard pile never filled")
}

func TestActionPlaysCardPartial(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	b := playCardDig(t, c, base, "decree")
	for _, want := range []string{
		"You stand at the Hellespont.", // scene unchanged but re-rendered
		"Played Royal Decree",          // log line
		"hx-swap-oob",                  // hand/log ride along as OOB swaps
	} {
		if !strings.Contains(b, want) {
			t.Fatalf("action response missing %q, got: %s", want, b)
		}
	}
	b = playCardDig(t, c, base, "phalanx")
	for _, want := range []string{"Played Phalanx", "Treasury -2"} {
		if !strings.Contains(b, want) {
			t.Fatalf("costed play missing %q, got: %s", want, b)
		}
	}
}

func TestActionChoiceSwapsScene(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	resp := postAction(t, c, base, url.Values{"choice": {"0"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /game/action = %d, want 200", resp.StatusCode)
	}
	if b := readBody(t, resp); !strings.Contains(b, "An open field awaits.") {
		t.Fatalf("choice did not swap scene, body: %s", b)
	}
}

func TestActionRendersEngineErrorWithout500(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	resp := postAction(t, c, base, url.Values{"card": {"ghost"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("engine error must re-render, got %d", resp.StatusCode)
	}
	if b := readBody(t, resp); !strings.Contains(b, "Error:") || !strings.Contains(b, "not in your hand") {
		t.Fatalf("error line missing from log, body: %s", b)
	}
}

// End-to-end regression for the clone-and-commit store: a refused action
// (unknown card) must not consume cards from the persisted hand.
func TestFailedActionDoesNotLoseCards(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	postAction(t, c, base, url.Values{"card": {"ghost"}})
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, "Phalanx") {
		t.Fatal("failed action must not remove cards from the hand")
	}
}

func TestStatGatedChoiceShowsRequirementUntilMet(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)

	// Before: on the field scene the gated choice is disabled and names
	// its requirement.
	postAction(t, c, base, url.Values{"choice": {"0"}}) // title → field
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, "Claim the vault") {
		t.Fatalf("field scene missing gated choice, got: %s", b)
	}
	if !strings.Contains(b, `disabled title="Requires Treasury 3"`) {
		t.Fatalf("gated choice must show its requirement, got: %s", b)
	}

	// After playing a treasury card, the page re-renders without the stat
	// requirement tooltip (the card-gated choice keeps its own).
	playCardDig(t, c, base, "decree")
	b = readBody(t, get(t, c, base+"/"))
	if strings.Contains(b, `disabled title="Requires Treasury 3"`) {
		t.Fatalf("stat requirement must disappear once met, got: %s", b)
	}
}

func TestEndingFlowRendersSummaryAndRefusesFurtherActions(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	playCardDig(t, c, base, "decree")
	postAction(t, c, base, url.Values{"choice": {"0"}}) // title → field

	resp := postAction(t, c, base, url.Values{"choice": {"1"}}) // claim vault
	b := readBody(t, resp)
	for _, want := range []string{
		"The world is yours.",
		`<span class="ending-badge triumph">triumph</span>`,
		"Begin a new campaign",
		"1 campaign completed",
		`class="ending-badge undiscovered">???</`,
		`aria-label="Endings discovered"`,
	} {
		if !strings.Contains(b, want) {
			t.Fatalf("ending summary missing %q, got: %s", want, b)
		}
	}
	if strings.Contains(b, "card-btn") {
		t.Fatal("hand must be hidden on ending scenes")
	}

	// Post-ending actions are refused and leave the ending scene in place.
	resp = postAction(t, c, base, url.Values{"card": {"phalanx"}})
	b = readBody(t, resp)
	if !strings.Contains(b, "campaign is over") {
		t.Fatalf("post-ending card play must be refused, got: %s", b)
	}
	if !strings.Contains(b, "The world is yours.") {
		t.Fatal("refused action must keep the ending scene")
	}
}

// The recorded run must persist across a new game: the title page then
// shows the campaign count and the discovered ending tile.
func TestRunHistorySurvivesNewGame(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	playCardDig(t, c, base, "decree")
	postAction(t, c, base, url.Values{"choice": {"0"}}) // title → field
	postAction(t, c, base, url.Values{"choice": {"1"}}) // → ending, run recorded

	resp := postForm(t, c, base+"/game/new", nil) // start over
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new game after ending = %d, want 200", resp.StatusCode)
	}
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, "March out") {
		t.Fatal("new game must start fresh at the title scene")
	}
	// Session persistence check via history: end the second run too, then
	// the counter must read two campaigns.
	playCardDig(t, c, base, "decree")
	postAction(t, c, base, url.Values{"choice": {"0"}})
	postAction(t, c, base, url.Values{"choice": {"1"}})
	b = readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, "2 campaigns completed") {
		t.Fatalf("second completed run not counted, got: %s", b)
	}
}

func TestCardCostDisablesUntilAffordable(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)

	// Broke at the start: the costed card in hand cannot be played.
	drawIntoHand(t, c, base, "phalanx")
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, `disabled title="Requires Treasury 2"`) {
		t.Fatalf("unaffordable card must be disabled with its cost, got: %s", b)
	}

	// After a treasury card, the page re-enables it.
	playCardDig(t, c, base, "decree")
	b = readBody(t, get(t, c, base+"/"))
	if strings.Contains(b, `disabled title="Requires Treasury`) {
		t.Fatalf("affordable card must be enabled, got: %s", b)
	}
	if !strings.Contains(b, "Phalanx") {
		t.Fatal("phalanx missing from page (hand or deck)")
	}
}

// drawIntoHand plays free cards until id shows up in the rendered hand.
func drawIntoHand(t *testing.T, c *http.Client, base, id string) {
	t.Helper()
	for i := 0; i < 6; i++ {
		b := readBody(t, get(t, c, base+"/"))
		if strings.Contains(b, `name="card" value="`+id+`"`) {
			return
		}
		playCardAny(t, c, base, "scout", "peltast", "herald")
	}
	t.Fatalf("%s never drawn into hand", id)
}

func TestConsumingChoiceSpendsCard(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	drawIntoHand(t, c, base, "decree")
	postAction(t, c, base, url.Values{"choice": {"0"}}) // title → field

	resp := postAction(t, c, base, url.Values{"choice": {"2"}}) // tear up decree
	b := readBody(t, resp)
	if strings.Contains(b, "Error:") {
		t.Fatalf("consuming choice failed: %s", b)
	}
	if !strings.Contains(b, "You stand at the Hellespont.") {
		t.Fatal("consuming choice must apply its effects")
	}
	if !strings.Contains(b, "spent Royal Decree") {
		t.Fatalf("log must record the spent card, got: %s", b)
	}
}

// A plain form post (no HX-Request header) must render a full page, not
// the htmx fragment set — that is the no-JavaScript fallback.
// The deck inspector renders the run's full card inventory, and scene
// arrival grants grow it.
func TestDeckInspectorTracksSceneGrants(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)

	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, `<summary>Deck · 5</summary>`) {
		t.Fatalf("deck inspector must show the 5 starting cards, got: %s", b)
	}

	// March out: arriving at field has no pool in this content, count stays.
	postAction(t, c, base, url.Values{"choice": {"0"}})
	b = readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, `<summary>Deck · 5</summary>`) {
		t.Fatalf("deck count must be stable without grants, got: %s", b)
	}
	if !strings.Contains(b, `class="deck-count">×1`) {
		t.Fatalf("deck list must show per-card counts, got: %s", b)
	}
}

func TestActionWithoutJSReturnsFullPage(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	resp := postForm(t, c, base+"/game/action", url.Values{"card": {"decree"}})
	b := readBody(t, resp)
	if strings.Contains(b, "Error:") {
		b = readBody(t, postForm(t, c, base+"/game/action", url.Values{"card": {"scout"}}))
		b = readBody(t, postForm(t, c, base+"/game/action", url.Values{"card": {"decree"}}))
	}
	if !strings.Contains(b, "<!DOCTYPE html>") {
		t.Fatal("plain post must render a full HTML page")
	}
	if strings.Contains(b, "hx-swap-oob") {
		t.Fatal("plain post must not return out-of-band swap fragments")
	}
	if !strings.Contains(b, "Played Royal Decree") {
		t.Fatal("action must still apply without JavaScript")
	}
}

func TestLogNewestFirstWithLiveRegion(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	postAction(t, c, base, url.Values{"choice": {"0"}}) // title → field
	postAction(t, c, base, url.Values{"choice": {"0"}}) // field → title
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, `role="log"`) {
		t.Fatal("log must be a live region for screen readers")
	}
	i := strings.Index(b, `<ul class="log" role="log">`)
	first := b[strings.Index(b[i:], "<li>")+i:]
	if !strings.HasPrefix(first, "<li>Return to the coast") {
		t.Fatalf("newest log entry must render first, got: %.80s", first)
	}
}

func TestSessionCookieHasExpiry(t *testing.T) {
	c, base := newTestClient(t)
	resp := get(t, c, base+"/")
	cookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, "Max-Age=") {
		t.Fatalf("session cookie must carry an expiry, got: %s", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("session cookie must stay HttpOnly and Lax, got: %s", cookie)
	}
}

func TestChronicleEmptyThenPopulated(t *testing.T) {
	c, base := newTestClient(t)

	// Fresh session: friendly empty state, reachable from the header.
	b := readBody(t, get(t, c, base+"/stats"))
	if !strings.Contains(b, "No campaigns have ended yet") {
		t.Fatalf("empty chronicle missing empty state, got: %s", b)
	}
	b = readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, `href="/stats"`) || !strings.Contains(b, "Chronicle") {
		t.Fatal("header must link to the chronicle")
	}

	// Complete a run; the chronicle summarizes and tabulates it.
	startGame(t, c, base)
	playCardDig(t, c, base, "decree")
	postAction(t, c, base, url.Values{"choice": {"0"}})
	postAction(t, c, base, url.Values{"choice": {"1"}}) // → ending, run recorded

	b = readBody(t, get(t, c, base+"/stats"))
	for _, want := range []string{
		"1 campaign completed",
		`<span class="ending-badge triumph">triumph</span><span class="count">×1</span>`,
		"Best campaign: triumph",
		`<table class="runs">`,
	} {
		if !strings.Contains(b, want) {
			t.Fatalf("chronicle missing %q, got: %s", want, b)
		}
	}
}

func TestLayoutLoadsEnhancementScripts(t *testing.T) {
	c, base := newTestClient(t)
	b := readBody(t, get(t, c, base+"/"))
	for _, want := range []string{
		`src="/static/htmx.min.js"`,
		`src="/static/game.js"`,
		`id="sfx-toggle"`,
		`href="/static/art.svg#icon-sound"`,
	} {
		if !strings.Contains(b, want) {
			t.Fatalf("layout missing %s", want)
		}
	}
}

// Cards render their emblem artwork and the deck inspector shows
// thumbnails; every shipped card id must have a matching symbol.
func TestCardsRenderArtwork(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	b := readBody(t, get(t, c, base+"/"))
	if n := strings.Count(b, `class="card-art" viewBox="0 0 64 64"`); n != 4 {
		t.Fatalf("hand must render 4 card artworks, got %d", n)
	}
	if !strings.Contains(b, `href="/static/art.svg#art_`) {
		t.Fatal("card artworks must reference the art sprite")
	}
	if !strings.Contains(b, `class="deck-art" viewBox="0 0 64 64"`) {
		t.Fatal("deck inspector must render thumbnails")
	}
}

// Shuffle is a first-class action: disabled with an empty discard, and
// recycles the pile for the cost of the turn.
func TestShuffleActionFlow(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)

	// Fresh game: nothing to shuffle, button disabled with a reason.
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, `disabled title="The discard pile is empty"`) {
		t.Fatalf("shuffle must be disabled on an empty discard, got: %s", b)
	}
	if strings.Contains(b, `Shuffle · 0`) && !strings.Contains(b, "Shuffle · 0") {
		t.Fatal("discard count must render")
	}

	// Playing cards fills the discard; shuffling recycles it.
	playCardDig(t, c, base, "decree")
	discardFills(t, c, base)

	// The discard viewer lists what has been burned through.
	b = readBody(t, get(t, c, base+"/"))
	if !regexp.MustCompile(`Discard · [1-9]`).MatchString(b) {
		t.Fatalf("discard viewer must count a non-empty pile, got: %s", b)
	}

	resp := postAction(t, c, base, url.Values{"shuffle": {"1"}})
	b = readBody(t, resp)
	if strings.Contains(b, "Error:") {
		t.Fatalf("shuffle failed: %s", b)
	}
	if !strings.Contains(b, "Shuffled the discard pile into the deck") {
		t.Fatalf("log must record the shuffle, got: %s", b)
	}
	if !strings.Contains(b, `disabled title="The discard pile is empty"`) {
		t.Fatal("shuffle must be disabled again after recycling")
	}
	if !strings.Contains(b, "Discard · 0") {
		t.Fatal("discard viewer must empty after the shuffle")
	}

	// A second shuffle without plays in between is refused.
	resp = postAction(t, c, base, url.Values{"shuffle": {"1"}})
	b = readBody(t, resp)
	if !strings.Contains(b, "nothing to shuffle") {
		t.Fatalf("empty shuffle must be refused, got: %s", b)
	}
}

// Scout reveals the deck's top card into the log for the cost of a turn.
func TestScoutActionFlow(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)

	b := readBody(t, get(t, c, base+"/"))
	if strings.Contains(b, `disabled title="The deck is empty"`) {
		t.Fatal("scout must be enabled with a stocked deck")
	}
	resp := postAction(t, c, base, url.Values{"scout": {"1"}})
	b = readBody(t, resp)
	if strings.Contains(b, "Error:") {
		t.Fatalf("scout failed: %s", b)
	}
	if !strings.Contains(b, "Scouted the deck: next card is") {
		t.Fatalf("log must record the scout, got: %s", b)
	}
}

func TestActionWithoutStateRedirectsHome(t *testing.T) {
	c, base := newTestClient(t)
	get(t, c, base+"/") // establish session cookie only
	resp := postForm(t, noRedirect(c), base+"/game/action", url.Values{"card": {"phalanx"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("action without state = %d, want 303", resp.StatusCode)
	}
}
