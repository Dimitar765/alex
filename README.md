# Alexander — a card adventure

A browser game about the life and legacy of Alexander the Great, mixing a
text adventure with a card game. Written in Go with server-rendered HTML
and htmx; no JavaScript required to play (htmx only enhances the UX).

```
go run ./cmd/gogame            # serves http://127.0.0.1:8080
go run ./cmd/gogame -addr :9000 -saves /tmp/saves
```

## How it plays

You march from Pella to Babylon through scenes with 2–3 choices. Three
stats track your run — **Legacy** (fame), **Army** (strength), **Treasury**
(gold). Choices may require stats or cards; some consume their card, and
battle choices resolve through weighted random outcomes. Cards in hand can
be played for their effects; powerful cards cost Treasury. **Scout** reveals
the deck's top card for a turn; **Shuffle** (both beside the deck
inspector) recycles the discard pile into the deck for a turn and 1
Treasury — the way to dig for a gate card you buried, with the discard
viewer showing what you burned through. Runs start
with a small Macedonian core deck that grows regionally as the campaign
reaches new lands. The run ends in one of five endings: triumph, legacy, settle,
or two flavors of defeat. Keys 1–3 take choices (with JavaScript); the
**Chronicle** page tracks your campaign history.

## Architecture

| Package | Role |
|---|---|
| `internal/content` | Data-driven schema (cards, scenes, effects) + full load-time validation |
| `internal/game` | Rules engine: state, choices, card play, costs, randomness |
| `internal/sim` | Headless random-greedy simulator: soft-lock proofs and ending distribution |
| `internal/web` | HTTP server: templates, htmx partials, cookie sessions, JSON saves |
| root (`assets`) | Embeds `content/` so the binary is self-contained |

Rules live only in `internal/game`; `internal/content` defines what data is
legal; `internal/web` only renders and persists. Every action runs on a
clone of the state and commits atomically (memory + save file together), so
a refused action or a crash can never corrupt a run.

## Authoring content

All content is JSON under `content/`. `content.Load` rejects, with a single
aggregated error, any dangling card/scene reference, unknown stat key,
malformed ending, unreachable scene, bad random weight, unobtainable card,
or fully gated scene — the server refuses to boot on invalid content.

**Cards** (`cards.json`): `id`, `name`, `text`, `cost` (Treasury to play,
optional), `start` (dealt into the opening deck), `effects[]`. Non-start
cards enter runs only through scene pools or `gain_card` effects; the
loader proves every card is obtainable that way.

**Scenes** (`scenes.json`): `id`, `text`, `cards` (regional pool granted on
first arrival), `ending` (optional; terminal scenes have no choices),
`choices[]` with `text`, optional `requiresCard`/`consumesCard`/
`requiresStat`, and `effects[]`. Every non-ending scene must keep at least
one ungated choice so a run can never soft-lock.

**Effects**: `stat` deltas (`{"legacy": 2}`), `gain_card`/`lose_card`
(to discard)/`remove_card` (exiled from the run), `goto` (arrival grants
the target scene's pool once), and `random` weighted `outcomes[]` whose
effects nest recursively.

Stats are a closed set: `legacy`, `army`, `treasury`. Endings are a closed
set: `triumph`, `legacy`, `settle`, `defeat`, `death`.

## Balance tooling

After editing content, simulate campaigns to check for soft-locks and
ending distribution:

```
go run ./cmd/simulate -runs 2000 -seed 1
```

The policy is random-greedy (uniform choices, occasional card plays), so
read defeat-heavy endings with that in mind — it deliberately picks the
mutiny, humans do not. The same seed always reproduces the same runs, and
`internal/sim` tests assert zero stuck runs and full ending reachability
on the shipped campaign.

## Presentation

The card table look is pure CSS + SVG + a dash of vanilla JS:

- **Artwork**: every card has a hand-drawn line-art emblem
  (`internal/web/static/art.svg`, a `<symbol>` sprite in pottery-style
  gold, referenced via `<use>`); the deck inspector shows thumbnails.
- **Animation**: cards deal in with a staggered slide, the played card
  lifts away while its request is held (`htmx:confirm`), siblings settle,
  the stat row pulses on change, and the ending badge pops. All of it
  respects `prefers-reduced-motion`.
- **Sound**: a tiny WebAudio synth in `game.js` (no audio files) —
  card flick, choice tick, error thud, and per-outcome ending chimes —
  with a header toggle persisted in `localStorage`.

Everything degrades: without JavaScript the game plays natively over
plain form posts, just without sound and motion.

## Saves

One JSON file per session in `saves/` (see `-saves` flag), written
atomically. Sessions idle for 30 days are swept at startup and daily.

## Testing

```
go test -race ./...
go vet ./...
```
