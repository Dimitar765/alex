// Command gogame serves the Alexander the Great card-adventure game on a
// local HTTP server. All content, templates, and assets are embedded.
package main

import (
	"flag"
	"log"

	"goGame/internal/content"
	"goGame/internal/web"

	assets "goGame"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	flag.Parse()

	lib, err := content.Load(assets.FS())
	if err != nil {
		log.Fatal(err)
	}
	srv := web.New(web.NewStore(lib, "saves"))
	srv.Addr = *addr
	log.Printf("gogame listening on http://%s", *addr)
	log.Fatal(srv.ListenAndServe())
}
