package instructions

import _ "embed"

//go:embed instructions.md
var instructionsText string

func Text(version string) string {
	return instructionsText
}
