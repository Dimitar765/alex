package web

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"goGame/internal/content"
)

var testContent = fstest.MapFS{
	"cards.json": &fstest.MapFile{Data: []byte(`[
      {"id":"phalanx","name":"Phalanx","text":"A wall of sarissas.","cost":2,"effects":[{"kind":"stat","delta":{"army":2}}]},
      {"id":"decree","name":"Royal Decree","text":"A seal, a scribble.","effects":[{"kind":"stat","delta":{"treasury":3}}]}
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

func TestActionPlaysCardPartial(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	// Decree is free and funds the treasury; the costed Phalanx then
	// becomes playable in the same partial flow.
	resp := postAction(t, c, base, url.Values{"card": {"decree"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /game/action = %d, want 200", resp.StatusCode)
	}
	b := readBody(t, resp)
	for _, want := range []string{
		"You stand at the Hellespont.", // scene unchanged but re-rendered
		"Played Royal Decree",          // log line
		"Treasury 3",                   // stat updated from 0
		"hx-swap-oob",                  // hand/log ride along as OOB swaps
	} {
		if !strings.Contains(b, want) {
			t.Fatalf("action response missing %q, got: %s", want, b)
		}
	}
	resp = postAction(t, c, base, url.Values{"card": {"phalanx"}})
	b = readBody(t, resp)
	for _, want := range []string{"Played Phalanx", "Treasury -2", "Army 2"} {
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

	// After playing a treasury card, the action partial re-renders the
	// choice without a requirement tooltip.
	resp := postAction(t, c, base, url.Values{"card": {"decree"}})
	b = readBody(t, resp)
	if strings.Contains(b, `title="Requires`) {
		t.Fatalf("requirement tooltip must disappear once met, got: %s", b)
	}
}

func TestEndingFlowRendersSummaryAndRefusesFurtherActions(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	postAction(t, c, base, url.Values{"card": {"decree"}})
	postAction(t, c, base, url.Values{"choice": {"0"}}) // title → field

	resp := postAction(t, c, base, url.Values{"choice": {"1"}}) // claim vault
	b := readBody(t, resp)
	for _, want := range []string{
		"The world is yours.",
		`<span class="ending-badge triumph">triumph</span>`,
		"Begin a new campaign",
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

func TestCardCostDisablesUntilAffordable(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)

	// Broke at the start: the costed card cannot be played.
	b := readBody(t, get(t, c, base+"/"))
	if !strings.Contains(b, `disabled title="Requires Treasury 2"`) {
		t.Fatalf("unaffordable card must be disabled with its cost, got: %s", b)
	}

	// After a treasury card, the action partial re-enables it.
	resp := postAction(t, c, base, url.Values{"card": {"decree"}})
	b = readBody(t, resp)
	if strings.Contains(b, `disabled title="Requires Treasury`) {
		t.Fatalf("affordable card must be enabled, got: %s", b)
	}
	if !strings.Contains(b, "Phalanx") {
		t.Fatal("phalanx missing from hand partial")
	}
}

func TestConsumingChoiceSpendsCard(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
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
func TestActionWithoutJSReturnsFullPage(t *testing.T) {
	c, base := newTestClient(t)
	startGame(t, c, base)
	resp := postForm(t, c, base+"/game/action", url.Values{"card": {"decree"}})
	b := readBody(t, resp)
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

func TestActionWithoutStateRedirectsHome(t *testing.T) {
	c, base := newTestClient(t)
	get(t, c, base+"/") // establish session cookie only
	resp := postForm(t, noRedirect(c), base+"/game/action", url.Values{"card": {"phalanx"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("action without state = %d, want 303", resp.StatusCode)
	}
}
