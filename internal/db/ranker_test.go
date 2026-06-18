package db

import (
	"testing"
	"time"
)

func TestCompositeScore_EqualRelevance_RecentHigher(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	weights := RankingWeights{
		RelevanceWeight:  0.4,
		RecencyWeight:    0.3,
		ImportanceWeight: 0.3,
		RecencyHalfLife:  30,
	}

	recent := CompositeScore(0.8, now, 0.5, weights)
	yesterdayScore := CompositeScore(0.8, yesterday, 0.5, weights)
	weekScore := CompositeScore(0.8, weekAgo, 0.5, weights)

	if recent <= yesterdayScore {
		t.Errorf("expected recent (%f) > yesterday (%f)", recent, yesterdayScore)
	}
	if yesterdayScore <= weekScore {
		t.Errorf("expected yesterday (%f) > week ago (%f)", yesterdayScore, weekScore)
	}
}

func TestCompositeScore_HighImportanceRanks(t *testing.T) {
	now := time.Now()
	weights := RankingWeights{
		RelevanceWeight:  0.4,
		RecencyWeight:    0.3,
		ImportanceWeight: 0.3,
		RecencyHalfLife:  30,
	}

	low := CompositeScore(0.8, now, 0.1, weights)
	high := CompositeScore(0.8, now, 0.9, weights)

	if high <= low {
		t.Errorf("expected high importance (%f) > low importance (%f)", high, low)
	}
}

func TestCompositeScore_RelevanceDominatesWhenWeightsFavor(t *testing.T) {
	now := time.Now()
	weights := RankingWeights{
		RelevanceWeight:  0.9,
		RecencyWeight:    0.05,
		ImportanceWeight: 0.05,
		RecencyHalfLife:  30,
	}

	highRel := CompositeScore(0.95, now, 0.1, weights)
	lowRel := CompositeScore(0.3, now, 0.9, weights)

	if highRel <= lowRel {
		t.Errorf("expected high relevance (%f) > low relevance (%f)", highRel, lowRel)
	}
}

func TestRankSearchResults_Reorders(t *testing.T) {
	now := time.Now()
	weekAgo := now.Add(-7 * 24 * time.Hour)

	results := []SearchResult{
		{Memory: &Memory{UpdatedAt: weekAgo, Importance: 0.9}, Score: 0.5},
		{Memory: &Memory{UpdatedAt: now, Importance: 0.1}, Score: 0.8},
	}

	weights := RankingWeights{
		RelevanceWeight:  0.4,
		RecencyWeight:    0.3,
		ImportanceWeight: 0.3,
		RecencyHalfLife:  30,
	}

	ranked := RankSearchResults(results, weights)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 results, got %d", len(ranked))
	}
	if ranked[0].Memory.Importance != 0.1 {
		t.Errorf("expected recent/low-importance first (relevance dominates), got importance=%f", ranked[0].Memory.Importance)
	}
}

func TestRankSearchResults_EmptyInput(t *testing.T) {
	weights := RankingWeights{}
	ranked := RankSearchResults(nil, weights)
	if ranked != nil {
		t.Errorf("expected nil output for nil input")
	}
}

func TestRankSearchResults_SingleResult(t *testing.T) {
	results := []SearchResult{
		{Memory: &Memory{UpdatedAt: time.Now(), Importance: 0.5}, Score: 0.8},
	}
	weights := RankingWeights{RelevanceWeight: 1.0}
	ranked := RankSearchResults(results, weights)
	if len(ranked) != 1 {
		t.Fatalf("expected 1 result, got %d", len(ranked))
	}
}
