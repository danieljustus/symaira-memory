package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

var (
	searchScope             string
	searchLimit             int
	searchEntity            string
	searchMinConfidence     string
	searchVerification      string
	searchExcludeSuperseded bool
	searchMaxAge            string
	searchMaxSensitivity    string
	searchMinSharingLevel   string
	searchClientID          string
	searchIncludeEmbedding  bool
)

func init() {
	searchCmd.Flags().StringVarP(&searchScope, "scope", "s", "", "Filter search by scope level")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 5, "Maximum number of search results to return")
	searchCmd.Flags().StringVar(&searchEntity, "entity", "", "Filter search by entity name")
	searchCmd.Flags().StringVar(&searchMinConfidence, "min-confidence", "", "Minimum confidence level: low, medium, high")
	searchCmd.Flags().StringVar(&searchVerification, "verification", "", "Filter by verification status: verified, unverified, stale")
	searchCmd.Flags().BoolVar(&searchExcludeSuperseded, "exclude-superseded", false, "Exclude memories that have been superseded")
	searchCmd.Flags().StringVar(&searchMaxAge, "max-age", "", "Maximum memory age (e.g. 7d, 30d, 1y)")
	searchCmd.Flags().StringVar(&searchMaxSensitivity, "max-sensitivity", "", "Maximum sensitivity level: public, internal, confidential, secret")
	searchCmd.Flags().StringVar(&searchMinSharingLevel, "min-sharing-level", "", "Minimum sharing level: private, team, org, public")
	searchCmd.Flags().StringVar(&searchClientID, "client-id", "", "Client ID for access control filtering")
	searchCmd.Flags().BoolVar(&searchIncludeEmbedding, "include-embedding", false, "Include raw embedding vectors in JSON output (omitted by default)")
	rootCmd.AddCommand(searchCmd)
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Perform semantic query comparison over stored memories offline",
	Example: `  # Search with default limit (5 results)
  symmemory search "preferred theme settings"

  # Return more results with --limit
  symmemory search "authentication flow" --limit 10

  # Filter by scope and entity
  symmemory search "API design decisions" -s project --entity "BackendAPI"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		embeddings := extractor.NewEmbeddingsGenerator(GetConfig())
		emb := embeddings.GenerateVector(query)

		var entityID string
		if searchEntity != "" {
			entity, err := GetDB().ResolveEntity(searchEntity)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "entity lookup error")
			}
			if entity == nil {
				return exitcodes.Wrapf(nil, exitcodes.ExitNotFound, exitcodes.KindNotFound, "entity not found: %s", searchEntity)
			}
			entityID = entity.ID
		}

		trustFilter := db.TrustFilter{
			MinConfidence:      searchMinConfidence,
			VerificationStatus: searchVerification,
			ExcludeSuperseded:  searchExcludeSuperseded,
		}
		if searchMaxAge != "" {
			dur, err := parseDuration(searchMaxAge)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "invalid max-age value")
			}
			trustFilter.MaxAge = dur
		}

		policyFilter := db.PolicyFilter{
			MaxSensitivity:  searchMaxSensitivity,
			MinSharingLevel: searchMinSharingLevel,
			ClientID:        searchClientID,
		}

		var results []db.SearchResult
		var err error

		if emb.Source == "hash-fallback" {
			results, err = GetDB().SearchMemoriesBM25(query, searchScope, searchLimit)
		} else {
			results, err = GetDB().SearchMemoriesFilteredWithTrust(emb.Vector, emb.Source, searchScope, searchLimit, entityID, trustFilter, policyFilter)
		}
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "semantic search failure")
		}

		formatter := NewOutputFormatter(GetOutputFormat(cmd))
		formatter.IncludeEmbedding = searchIncludeEmbedding
		if err := formatter.Output(results, "search"); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "output error")
		}
		return nil
	},
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}
	suffix := s[len(s)-1]
	switch suffix {
	case 'd':
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h':
		return time.ParseDuration(s)
	case 'm':
		return time.ParseDuration(s)
	case 's':
		return time.ParseDuration(s)
	default:
		return time.ParseDuration(s)
	}
}
