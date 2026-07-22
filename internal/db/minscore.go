package db

// FilterByMinScore drops results whose similarity score is below minScore.
// A minScore <= 0 disables filtering and returns the input unchanged.
func FilterByMinScore(results []SearchResult, minScore float64) []SearchResult {
	if minScore <= 0 {
		return results
	}
	kept := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if float64(r.Score) >= minScore {
			kept = append(kept, r)
		}
	}
	return kept
}
