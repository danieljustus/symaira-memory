package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const maxCandidates = 2000

// allowedFTSScopes is the set of scope values permitted in FTS5 queries.
// Must match security.ValidScopes to prevent FTS5 injection via the scope parameter.
var allowedFTSScopes = map[string]bool{
	"":        true,
	"global":  true,
	"project": true,
	"agent":   true,
	"user":    true,
	"session": true,
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
	"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "shall": true,
	"can": true, "to": true, "of": true, "in": true, "for": true, "on": true,
	"with": true, "at": true, "by": true, "from": true, "as": true, "into": true,
	"through": true, "during": true, "before": true, "after": true, "above": true,
	"below": true, "between": true, "out": true, "off": true, "over": true,
	"under": true, "again": true, "further": true, "then": true, "once": true,
	"and": true, "but": true, "or": true, "nor": true, "not": true, "so": true,
	"yet": true, "both": true, "either": true, "neither": true, "each": true,
	"every": true, "all": true, "any": true, "few": true, "more": true,
	"most": true, "other": true, "some": true, "such": true, "no": true,
	"only": true, "own": true, "same": true, "than": true, "too": true,
	"very": true, "just": true, "because": true, "if": true, "when": true,
	"where": true, "how": true, "what": true, "which": true, "who": true,
	"whom": true, "this": true, "that": true, "these": true, "those": true,
	"i": true, "me": true, "my": true, "we": true, "our": true, "you": true,
	"your": true, "he": true, "him": true, "his": true, "she": true, "her": true,
	"it": true, "its": true, "they": true, "them": true, "their": true,
}

// Tokenize splits text into lowercased tokens, filtering stop words and short words.
func Tokenize(text string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			word := current.String()
			current.Reset()
			if !stopWords[word] && len(word) > 1 {
				tokens = append(tokens, word)
			}
		}
	}
	if current.Len() > 0 {
		word := current.String()
		if !stopWords[word] && len(word) > 1 {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

// HybridResult extends SearchResult with hybrid scoring details.
type HybridResult struct {
	Memory      *Memory `json:"memory"`
	VectorScore float32 `json:"vector_score"`
	BM25Score   float64 `json:"bm25_score"`
	FusedScore  float64 `json:"fused_score"`
}

// ReciprocalRankFusion combines ranked lists using RRF: score = sum(1/(k+rank_i)).
// k=60 is the standard constant.
func ReciprocalRankFusion(rankedLists [][]string, k int) map[string]float64 {
	if k <= 0 {
		k = 60
	}
	scores := make(map[string]float64)
	for _, list := range rankedLists {
		for rank, id := range list {
			scores[id] += 1.0 / float64(k+rank+1)
		}
	}
	return scores
}

// SearchMemoriesBM25 performs keyword-based search using SQLite FTS5 BM25 scoring.
func (db *DB) SearchMemoriesBM25(query string, scope string, limit int) ([]SearchResult, error) {
	if !allowedFTSScopes[scope] {
		return nil, fmt.Errorf("invalid search scope %q: must be one of global, project, agent, user, session", scope)
	}

	queryTerms := Tokenize(query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	var ftsQuery string
	if scope != "" {
		ftsQuery = "scope:" + scope + " AND (" + strings.Join(queryTerms, " OR ") + ")"
	} else {
		ftsQuery = strings.Join(queryTerms, " OR ")
	}

	rows, err := db.conn.Query(
		`SELECT m.id, m.content, m.scope, m.metadata, m.created_at, m.updated_at,
		        m.created_by, m.updated_by, m.created_session, m.updated_session,
		        m.consolidation_status, m.consolidated_into_id, m.importance,
		        m.valid_from, m.valid_to, m.superseded_by,
		        rank
		 FROM memories_fts fts
		 JOIN memories m ON fts.id = m.id
		 WHERE memories_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		ftsQuery, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var m Memory
		var metaStr string
		var consolidatedInto sql.NullString
		var validFrom, validTo sql.NullTime
		var supersededBy sql.NullString
		var rank float64
		if err := rows.Scan(&m.ID, &m.Content, &m.Scope, &metaStr, &m.CreatedAt, &m.UpdatedAt,
			&m.CreatedBy, &m.UpdatedBy, &m.CreatedSession, &m.UpdatedSession,
			&m.ConsolidationStatus, &consolidatedInto, &m.Importance,
			&validFrom, &validTo, &supersededBy, &rank); err != nil {
			return nil, err
		}
		if err := populateMemoryFields(&m, metaStr, consolidatedInto, validFrom, validTo, supersededBy); err != nil {
			return nil, err
		}
		results = append(results, SearchResult{
			Memory: &m,
			Score:  float32(-rank),
		})
	}

	return results, rows.Err()
}

// HybridSearch combines vector similarity and BM25 keyword search using
// Reciprocal Rank Fusion. Returns results ranked by fused score.
func (db *DB) HybridSearch(queryVec []float32, querySource string, queryText string, scope string, limit int, vectorWeight, bm25Weight float64) ([]HybridResult, error) {
	candidateLimit := limit * 3
	if candidateLimit > maxCandidates {
		candidateLimit = maxCandidates
	}

	vectorResults, err := db.SearchMemories(queryVec, querySource, scope, candidateLimit)
	if err != nil {
		return nil, err
	}

	bm25Results, err := db.SearchMemoriesBM25(queryText, scope, candidateLimit)
	if err != nil {
		return nil, err
	}

	var vectorList, bm25List []string
	for _, r := range vectorResults {
		vectorList = append(vectorList, r.Memory.ID)
	}
	for _, r := range bm25Results {
		bm25List = append(bm25List, r.Memory.ID)
	}

	rrfScores := ReciprocalRankFusion([][]string{vectorList, bm25List}, 60)

	memByID := make(map[string]*Memory)
	vecScoreByID := make(map[string]float32)
	for _, r := range vectorResults {
		memByID[r.Memory.ID] = r.Memory
		vecScoreByID[r.Memory.ID] = r.Score
	}
	bm25ScoreByID := make(map[string]float64)
	for _, r := range bm25Results {
		memByID[r.Memory.ID] = r.Memory
		bm25ScoreByID[r.Memory.ID] = float64(r.Score)
	}

	type fused struct {
		id         string
		fusedScore float64
	}
	var all []fused
	for id, rrf := range rrfScores {
		vecS := float64(vecScoreByID[id])
		bm25S := bm25ScoreByID[id]
		fusedScore := vectorWeight*vecS + bm25Weight*bm25S + (1-vectorWeight-bm25Weight)*rrf
		all = append(all, fused{id: id, fusedScore: fusedScore})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].fusedScore > all[j].fusedScore
	})

	if limit > len(all) {
		limit = len(all)
	}

	var results []HybridResult
	for i := 0; i < limit; i++ {
		id := all[i].id
		results = append(results, HybridResult{
			Memory:      memByID[id],
			VectorScore: vecScoreByID[id],
			BM25Score:   bm25ScoreByID[id],
			FusedScore:  all[i].fusedScore,
		})
	}
	return results, nil
}
