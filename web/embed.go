// Package web bettet den Frontend-Build (web/dist) in das Go-Binary ein.
package web

import (
	"embed"
	"io/fs"
)

// dist enthält mindestens .gitkeep, damit das Paket auch ohne Frontend-Build
// kompiliert; der Server zeigt dann eine Hinweisseite.
//
//go:embed all:dist
var dist embed.FS

// Dist liefert den Inhalt von web/dist als Wurzel-Dateisystem.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
