package db

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

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

func tokenize(text string) []string {
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

type bm25Doc struct {
	id     string
	terms  map[string]int // term → count
	docLen int
}

type bm25Index struct {
	docs      map[string]*bm25Doc
	docFreqs  map[string]int // term → number of docs containing it
	totalDocs int
	avgDocLen float64
}

func newBM25Index() *bm25Index {
	return &bm25Index{
		docs:     make(map[string]*bm25Doc),
		docFreqs: make(map[string]int),
	}
}

func (idx *bm25Index) addDoc(id, content string) {
	terms := tokenize(content)
	termCounts := make(map[string]int)
	for _, t := range terms {
		termCounts[t]++
	}
	idx.docs[id] = &bm25Doc{
		id:     id,
		terms:  termCounts,
		docLen: len(terms),
	}
	for t := range termCounts {
		idx.docFreqs[t]++
	}
	idx.totalDocs++
	var totalLen int
	for _, d := range idx.docs {
		totalLen += d.docLen
	}
	idx.avgDocLen = float64(totalLen) / float64(idx.totalDocs)
}

// BM25 scoring with k1=1.5, b=0.75
func (idx *bm25Index) score(queryTerms []string) map[string]float64 {
	const k1 = 1.5
	const b = 0.75
	scores := make(map[string]float64)
	for id, doc := range idx.docs {
		var score float64
		for _, qt := range queryTerms {
			tf, ok := doc.terms[qt]
			if !ok {
				continue
			}
			df := idx.docFreqs[qt]
			idf := math.Log((float64(idx.totalDocs)-float64(df)+0.5)/(float64(df)+0.5) + 1.0)
			tfNorm := (float64(tf) * (k1 + 1)) / (float64(tf) + k1*(1-b+b*float64(doc.docLen)/idx.avgDocLen))
			score += idf * tfNorm
		}
		if score > 0 {
			scores[id] = score
		}
	}
	return scores
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

// SearchMemoriesBM25 performs keyword-based search using BM25 scoring.
func (db *DB) SearchMemoriesBM25(query string, scope string, limit int) ([]SearchResult, error) {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	var memories []*Memory
	var err error
	if scope != "" {
		memories, err = db.ListMemoriesLite(scope, 0, 5000)
	} else {
		memories, err = db.ListMemoriesLite("", 0, 5000)
	}
	if err != nil {
		return nil, err
	}
	if len(memories) == 0 {
		return nil, nil
	}

	idx := newBM25Index()
	for _, m := range memories {
		idx.addDoc(m.ID, m.Content)
	}

	bm25Scores := idx.score(queryTerms)
	type scored struct {
		id    string
		score float64
	}
	var ranked []scored
	for id, s := range bm25Scores {
		ranked = append(ranked, scored{id: id, score: s})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	if limit > len(ranked) {
		limit = len(ranked)
	}

	memByID := make(map[string]*Memory, len(memories))
	for _, m := range memories {
		memByID[m.ID] = m
	}

	var results []SearchResult
	for i := 0; i < limit; i++ {
		results = append(results, SearchResult{
			Memory: memByID[ranked[i].id],
			Score:  float32(ranked[i].score),
		})
	}
	return results, nil
}

// HybridSearch combines vector similarity and BM25 keyword search using
// Reciprocal Rank Fusion. Returns results ranked by fused score.
func (db *DB) HybridSearch(queryVec []float32, queryText string, scope string, limit int, vectorWeight, bm25Weight float64) ([]HybridResult, error) {
	vectorResults, err := db.SearchMemories(queryVec, scope, limit*3)
	if err != nil {
		return nil, err
	}

	bm25Results, err := db.SearchMemoriesBM25(queryText, scope, limit*3)
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
