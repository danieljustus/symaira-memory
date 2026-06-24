package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/spf13/cobra"
)

var (
	syncRemote string
	syncToken  string
)

func init() {
	syncCmd.Flags().StringVar(&syncRemote, "remote", "", "Remote server base URL (e.g. http://localhost:8787)")
	syncCmd.Flags().StringVar(&syncToken, "token", "", "JWT bearer token for authentication")
	_ = syncCmd.MarkFlagRequired("remote")
	_ = syncCmd.MarkFlagRequired("token")
	rootCmd.AddCommand(syncCmd)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Bidirectional sync with a remote Symaira Memory server (pull + push with LWW merge)",
	Run: func(cmd *cobra.Command, args []string) {
		database := GetDB()
		if database == nil {
			fmt.Fprintf(os.Stderr, "Error: database not initialized\n")
			os.Exit(1)
			return
		}

		if err := runSync(database, syncRemote, syncToken); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func runSync(database *db.DB, remote, token string) error {
	remote = strings.TrimRight(remote, "/")
	client := &http.Client{Timeout: 30 * time.Second}
	embeddings := extractor.NewEmbeddingsGenerator(nil)

	cursor, err := database.GetSyncCursor(remote)
	if err != nil {
		return fmt.Errorf("reading sync cursor: %w", err)
	}

	// Pull: drain every remote page before advancing the local cursor.
	var allMemories []*db.Memory
	var serverTimeStr string
	cursorParam := ""
	if !cursor.IsZero() {
		cursorParam = "?since=" + cursor.UTC().Format(time.RFC3339)
	}

	for {
		pullURL := remote + "/api/sync/changes" + cursorParam
		if !strings.Contains(pullURL, "?") {
			pullURL += "?include_embeddings=true"
		} else {
			pullURL += "&include_embeddings=true"
		}

		pullReq, err := http.NewRequest("GET", pullURL, nil)
		if err != nil {
			return fmt.Errorf("building pull request: %w", err)
		}
		pullReq.Header.Set("Authorization", "Bearer "+token)

		pullResp, err := client.Do(pullReq)
		if err != nil {
			return fmt.Errorf("contacting remote: %w", err)
		}

		if pullResp.StatusCode == http.StatusUnauthorized {
			pullResp.Body.Close()
			return fmt.Errorf("authentication failed (401). Check your --token")
		}
		if pullResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(pullResp.Body)
			pullResp.Body.Close()
			return fmt.Errorf("pull failed with status %d: %s", pullResp.StatusCode, string(body))
		}

		var pullResult struct {
			Memories   []*db.Memory `json:"memories"`
			ServerTime string       `json:"server_time"`
			NextCursor string       `json:"next_cursor"`
		}
		if err := json.NewDecoder(pullResp.Body).Decode(&pullResult); err != nil {
			pullResp.Body.Close()
			return fmt.Errorf("decoding pull response: %w", err)
		}
		pullResp.Body.Close()

		allMemories = append(allMemories, pullResult.Memories...)
		serverTimeStr = pullResult.ServerTime

		if pullResult.NextCursor == "" {
			break
		}
		cursorParam = "?cursor=" + pullResult.NextCursor
	}

	serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
	if err != nil {
		return fmt.Errorf("parsing server_time: %w", err)
	}

	appliedCount := 0
	skippedCount := 0
	for _, m := range allMemories {
		if m.ID == "" {
			skippedCount++
			continue
		}
		ok, err := database.UpsertMemoryIfNewer(m)
		if err != nil {
			return fmt.Errorf("applying memory %s: %w", m.ID, err)
		}
		if ok {
			appliedCount++
		} else {
			skippedCount++
		}
	}

	for _, m := range allMemories {
		if m.ID == "" || len(m.Embedding) > 0 {
			continue
		}
		vec := embeddings.GenerateVector(m.Content)
		if err := database.SetMemoryEmbedding(m.ID, vec.Vector, vec.Source, vec.Model); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to backfill embedding for %s: %v\n", m.ID, err)
		}
	}

	localChanges, err := database.GetMemoriesSinceCursor(cursor, 0, true)
	if err != nil {
		return fmt.Errorf("reading local changes: %w", err)
	}

	pushedCount := 0
	if len(localChanges) > 0 {
		payload, err := json.Marshal(map[string][]*db.Memory{
			"memories": localChanges,
		})
		if err != nil {
			return fmt.Errorf("encoding push payload: %w", err)
		}

		pushReq, err := http.NewRequest("POST", remote+"/api/sync/apply", bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("building push request: %w", err)
		}
		pushReq.Header.Set("Authorization", "Bearer "+token)
		pushReq.Header.Set("Content-Type", "application/json")

		pushResp, err := client.Do(pushReq)
		if err != nil {
			return fmt.Errorf("pushing to remote: %w", err)
		}
		defer pushResp.Body.Close()

		if pushResp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("authentication failed (401) on push. Check your --token")
		}
		if pushResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(pushResp.Body)
			return fmt.Errorf("push failed with status %d: %s", pushResp.StatusCode, string(body))
		}

		var pushResult struct {
			Applied int `json:"applied"`
			Skipped int `json:"skipped"`
		}
		if err := json.NewDecoder(pushResp.Body).Decode(&pushResult); err != nil {
			return fmt.Errorf("decoding push response: %w", err)
		}
		pushedCount = pushResult.Applied
	}

	// Advance cursor only after a complete successful pull.
	if err := database.SetSyncCursor(remote, serverTime); err != nil {
		return fmt.Errorf("saving sync cursor: %w", err)
	}

	fmt.Printf("pulled: %d applied / %d skipped, pushed: %d, cursor: %s\n",
		appliedCount, skippedCount, pushedCount, serverTime.UTC().Format(time.RFC3339))
	return nil
}
