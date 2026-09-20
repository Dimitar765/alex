// Command simulate plays automated campaigns against the shipped content
// and prints the ending distribution, for balance work after content edits.
package main

import (
	"flag"
	"fmt"
	"log"

	"goGame/internal/content"
	"goGame/internal/sim"

	assets "goGame"
)

func main() {
	runs := flag.Int("runs", 1000, "number of campaigns to simulate")
	seed := flag.Uint64("seed", 1, "random seed (same seed, same results)")
	flag.Parse()

	lib, err := content.Load(assets.FS())
	if err != nil {
		log.Fatal(err)
	}
	report := sim.Aggregate(sim.Simulate(lib, *runs, *seed))
	fmt.Print(report)
}
