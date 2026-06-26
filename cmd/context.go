package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/contextassembler"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/spf13/cobra"
)

var (
	contextQuery  string
	contextBudget int
	contextFormat string
	contextScope  string
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Emit a token-budgeted context block from the memory store",
	Long: `Assemble a context block from the memory store, suitable for injection
into an AI agent prompt. The output is token-budgeted and available in
Markdown or JSON format. When the database is unavailable or empty, an
empty/minimal block is emitted so that hooks never break.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			emitContextEmpty(contextFormat)
			return nil
		}

		scope := contextScope
		if scope == "" {
			scope = security.NewProjectScopeDetector().DetectActiveProject()
		}

		budget := contextBudget
		if budget <= 0 {
			budget = cfg.Context.TokenBudget
		}

		database, err := db.Open(cfg)
		if err != nil {
			emitContextEmpty(contextFormat)
			return nil
		}
		defer database.Close()

		embeddings := extractor.NewEmbeddingsGenerator(cfg)

		ctxCfg := cfg.Context
		if budget > 0 {
			ctxCfg.TokenBudget = budget
		}
		assembler := contextassembler.NewAssembler(database, embeddings, &ctxCfg)

		result, err := assembler.Assemble(contextQuery, "", "")
		if err != nil {
			emitContextEmpty(contextFormat)
			return nil
		}

		if contextQuery == "" && len(result.Pieces) == 0 {
			recentContent := assembleRecentMemories(database, scope, result.Budget)
			if recentContent != "" {
				tokens := estimateContextTokens(recentContent)
				result.Pieces = append(result.Pieces, contextassembler.AssembledPiece{
					Layer:   contextassembler.LayerRetrieval,
					Content: recentContent,
					Tokens:  tokens,
				})
				result.UsedTokens += tokens
			}
		}

		if contextFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		for _, piece := range result.Pieces {
			fmt.Println(piece.Content)
		}
		return nil
	},
}

func init() {
	contextCmd.Flags().StringVar(&contextQuery, "query", "", "Semantic query to drive retrieval")
	contextCmd.Flags().IntVar(&contextBudget, "budget", 0, "Token budget (0 = use config default)")
	contextCmd.Flags().StringVar(&contextFormat, "format", "md", "Output format: md or json")
	contextCmd.Flags().StringVar(&contextScope, "scope", "", "Scope level (auto-detected if empty)")
	rootCmd.AddCommand(contextCmd)
}

func emitContextEmpty(format string) {
	if format == "json" {
		fmt.Println(`{"query":"","budget":0,"used_tokens":0,"pieces":[]}`)
	}
	// Markdown: empty block = no output.
}

func estimateContextTokens(text string) int {
	if text == "" {
		return 0
	}
	words := strings.Fields(text)
	return (len(words) * 4) / 3
}

func assembleRecentMemories(database *db.DB, scope string, budget int) string {
	mems, err := database.ListMemoriesFiltered(scope, "", 0, 20)
	if err != nil || len(mems) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Recent project memories:\n")
	used := 0
	for _, m := range mems {
		line := fmt.Sprintf("- %s\n", m.Content)
		tokens := estimateContextTokens(line)
		if used+tokens > budget {
			break
		}
		sb.WriteString(line)
		used += tokens
	}
	return sb.String()
}
