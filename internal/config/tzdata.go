package config

// Embed the IANA database so standalone Windows binaries can validate and use
// configured timezone names without depending on a Go installation or host
// zoneinfo files.
import _ "time/tzdata"
