// Command alexander-ebitengine is the native Ebitengine client: the same
// game logic and content as the web build, rendered as a desktop (and
// eventually WASM) application without a browser layer.
package main

import (
	"log"

	"goGame/internal/desktop"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	ebiten.SetWindowSize(desktop.ScreenW, desktop.ScreenH)
	ebiten.SetWindowTitle("Alexander — a card adventure")
	if err := ebiten.RunGame(desktop.New()); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
