package main

import "embed"

// webFS trägt das komplette Frontend im Binary: App-Shell, Skripte, Styles,
// Icons, Manifest und Service Worker. Damit bleibt drop ein einziges Binary
// ohne Dateien daneben.
//
//go:embed all:web
var webFS embed.FS
