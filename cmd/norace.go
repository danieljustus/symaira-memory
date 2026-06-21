//go:build !race

package cmd

func raceEnabled() bool { return false }
