// Package fonts embeds the bundled typefaces and their licenses.
// EB Garamond (body, plus italic) and Cinzel (display headers) are
// variable TTFs under the SIL Open Font License.
package fonts

import (
	"embed"
)

//go:embed EBGaramond.ttf EBGaramond-Italic.ttf Cinzel.ttf LICENSE-EBGaramond.txt LICENSE-Cinzel.txt
var files embed.FS

// MustOpen returns a reader over the named embedded file, panicking on
// failure (the names are compile-time constants of this package).
func MustOpen(name string) []byte {
	b, err := files.ReadFile(name)
	if err != nil {
		panic("fonts: " + err.Error())
	}
	return b
}
