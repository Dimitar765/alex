# Alexander — a card adventure

A native 3D game about the life and legacy of Alexander the Great, mixing a
text adventure with a card game. Written in Rust on Bevy; one binary runs
on Windows, macOS, and Linux with every asset embedded.

```
cargo run -p alexander --release            # play
cargo run -p alexander-core --bin simulate -- --runs 2000 --seed 1   # balance
cargo test --workspace                      # core suites
cargo clippy --workspace -- -D warnings     # lint
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
or two flavors of defeat. Keys 1–3 take choices; the **Chronicle** page
tracks your campaign history.

### The Persian response

After every turn you spend — cards, choices, scout, shuffle — the Great
King's host advances: **+1 base plus the pressure of the region you stand
in** (fronts press hardest, home cities not at all). The track is
deterministic, so you can plan around it. At **8** the Persians raid your
baggage train (−1 Treasury); at **14** an ambush scatters a random hand
card into the discard; at **20** the campaign erupts into a final battle
you cannot march past — hold it with a strong Army, charge it with
Bucephalus, buy your way out with 5 Treasury, or slink away in defeat.
Some cards calm the front instead of fighting: spending them for
`Threat −2` is often the wiser war.

## Architecture

| Crate | Role |
|---|---|
| `crates/core` (`alexander-core`) | Content schema + validation (`content.rs`), rules engine (`game.rs`), view-model/FX-diff/saves (`app/`), balance simulator (`sim.rs`, `bin/simulate`) |
| `crates/client` (`alexander`) | The Bevy game: 3D hand + table (`hand3d.rs`, `paint.rs`, `tween.rs`), UI screens (`screens/`), FX (`fx.rs`), synth audio (`audio.rs`), input incl. gamepad (`input.rs`) |

Rules live only in `crates/core/src/game.rs`; `content.rs` defines what
data is legal; the client only renders and persists. Every action runs on
a clone of the state and commits atomically (memory + save file together),
so a refused action or a crash can never corrupt a run. Content JSON,
fonts, and card art are `include_str!`/`include_bytes!`-embedded — release
binaries are self-contained.

## Authoring content

All content is JSON under `content/`. Loading rejects, with a single
aggregated error, any dangling card/scene reference, unknown stat key,
malformed ending, unreachable scene, bad random weight, unobtainable card,
or fully gated scene — the game refuses to boot on invalid content.

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
cargo run -p alexander-core --bin simulate -- --runs 2000 --seed 1
```

The policy is random-greedy (uniform choices, occasional card plays), so
read defeat-heavy endings with that in mind — it deliberately picks the
mutiny, humans do not. The same seed always reproduces the same runs, and
the `sim` tests assert zero stuck runs and full ending reachability on the
shipped campaign. The binary exits 1 if any run soft-locked.

## Presentation

The card table is a real 3D scene: the hand renders as lit card meshes —
fronts painted at runtime from the gold emblem PNGs with name, flavor, and
cost chip, meander-pattern backs — fanned on a table with a deck stack,
staggered deal-ins, hover lift via raycasting, play arcs toward the scene
panel, a shuffle riffle, and a scout deck-pulse. The camera parallaxes with
the pointer, dollies to keep the fan framed on resize, and renders through
filmic tonemapping with a subtle bloom so the gold reads. A hand-rolled
tween engine drives it all (cubic ease-out, per-property kill), collapsing
to single-frame steps under reduced motion.

An FX layer (hand-rolled particles, zero new deps) dusts the table: shuffle
throws ghost cards with dust, card plays trail gold motes toward the scene,
stat deltas float as +N/−N, dice rolls micro-shake the scene with dust, and
endings get rising gold or falling embers plus a vignette. **M** mutes, **F**
toggles reduced motion, and **X** switches the FX layer off; all persist in
`settings.json` beside the saves.

**Sound** is a tiny runtime synth (no audio files) — card flick, choice
tick, shuffle rustle, scout ping, error thud, and per-outcome ending
chimes, rendered to WAV in memory from the same parameter table as the
original clients.

**Input**: keyboard (digits choose, arrows + Enter navigate, Esc backs
out), mouse (hover lift, click to play), and gamepad (D-pad/stick + A/B,
any pad acts, 0.5 deadzone).

## Saves

One JSON save plus a run history under the OS config dir
(`%APPDATA%\Alexander\saves`, `~/Library/Application Support/Alexander/saves`,
`$XDG_CONFIG_HOME/Alexander/saves`), written atomically. Corrupt saves read
as empty and land you on the title screen, never an error. Settings live
beside the saves in `settings.json`.

## Testing

```
cargo test --workspace
cargo clippy --workspace -- -D warnings
```

The core suites pin log wording, refusal semantics, threat-edge cases, FX
ordering, save-layout compatibility, and simulator guarantees.
