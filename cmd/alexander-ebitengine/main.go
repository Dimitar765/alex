// Command alexander-ebitengine is the native Ebitengine client: the same
// game logic and content as the web build, rendered as a desktop (and
// eventually WASM) application without a browser layer.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"goGame/internal/desktop"
)

func main() {
	g, err := desktop.New()
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowSize(desktop.ScreenW, desktop.ScreenH)
	ebiten.SetWindowTitle("Alexander — a card adventure")
	if err := ebiten.RunGame(g); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
