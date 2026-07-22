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

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/spf13/cobra"
)

var (
	syncRemote   string
	syncToken    string
	syncPushOnly bool
	syncPullOnly bool
	syncRelay    bool
	syncPassFile string
)

func init() {
	syncCmd.Flags().StringVar(&syncRemote, "remote", "", "Remote server base URL (e.g. http://localhost:8787)")
	syncCmd.Flags().StringVar(&syncToken, "token", "", "JWT bearer token for authentication")
	syncCmd.Flags().BoolVar(&syncPushOnly, "push-only", false, "Only push local changes; do not apply remote changes")
	syncCmd.Flags().BoolVar(&syncPullOnly, "pull-only", false, "Only apply remote changes; do not push local changes")
	syncCmd.Flags().BoolVar(&syncRelay, "relay", false, "Use the encrypted relay endpoint; the remote only stores ciphertext (requires --passphrase-file)")
	syncCmd.Flags().StringVar(&syncPassFile, "passphrase-file", "", "File containing the encryption passphrase for --relay")
	_ = syncCmd.MarkFlagRequired("remote")
	_ = syncCmd.MarkFlagRequired("token")
	syncCmd.MarkFlagsMutuallyExclusive("push-only", "pull-only")
	rootCmd.AddCommand(syncCmd)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Bidirectional sync with a remote Symaira Memory server (pull + push with LWW merge)",
	RunE: func(cmd *cobra.Command, args []string) error {
		database := GetDB()
		if database == nil {
			return exitcodes.Wrapf(nil, exitcodes.ExitSoftware, exitcodes.KindInternal, "database not initialized")
		}

		if syncRelay {
			if syncPassFile == "" {
				return exitcodes.Wrapf(nil, exitcodes.ExitNoInput, exitcodes.KindValidation, "--relay requires --passphrase-file")
			}
			passphrase, err := readPassphraseFile(syncPassFile)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitNoInput, exitcodes.KindValidation, "reading passphrase file")
			}
			if err := runRelaySync(database, syncRemote, syncToken, passphrase); err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "relay sync failed")
			}
			return nil
		}

		if err := runSync(database, syncRemote, syncToken); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "sync failed")
		}
		return nil
	},
}

func readPassphraseFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	passphrase := strings.TrimSpace(string(data))
	if passphrase == "" {
		return "", fmt.Errorf("passphrase file is empty")
	}
	return passphrase, nil
}

func syncHTTPDo(client *http.Client, method, url, token string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("building %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting remote: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, fmt.Errorf("authentication failed (401). Check your --token")
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s failed with status %d: %s", method, url, resp.StatusCode, string(respBody))
	}
	return resp, nil
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
	var allDeleted []db.DeletedMemory
	serverTimeStr := ""
	appliedCount := 0
	skippedCount := 0
	deletedCount := 0

	if !syncPushOnly {
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

			pullResp, err := syncHTTPDo(client, "GET", pullURL, token, nil)
			if err != nil {
				return err
			}

			var pullResult struct {
				Memories   []*db.Memory       `json:"memories"`
				Deleted    []db.DeletedMemory `json:"deleted"`
				ServerTime string             `json:"server_time"`
				NextCursor string             `json:"next_cursor"`
			}
			if err := json.NewDecoder(pullResp.Body).Decode(&pullResult); err != nil {
				pullResp.Body.Close()
				return fmt.Errorf("decoding pull response: %w", err)
			}
			pullResp.Body.Close()

			allMemories = append(allMemories, pullResult.Memories...)
			allDeleted = append(allDeleted, pullResult.Deleted...)
			serverTimeStr = pullResult.ServerTime

			if pullResult.NextCursor == "" {
				break
			}
			cursorParam = "?cursor=" + pullResult.NextCursor
		}

		for _, m := range allMemories {
			if m.ID == "" {
				skippedCount++
				continue
			}
			ok, err := database.SyncUpsertMemoryIfNewer(m)
			if err != nil {
				return fmt.Errorf("applying memory %s: %w", m.ID, err)
			}
			if ok {
				appliedCount++
			} else {
				skippedCount++
			}
		}

		for _, d := range allDeleted {
			if d.ID == "" {
				continue
			}
			removed, err := database.ApplyRemoteDelete(d.ID, d.DeletedAt)
			if err != nil {
				return fmt.Errorf("applying delete %s: %w", d.ID, err)
			}
			if removed {
				deletedCount++
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
	}

	pushedCount := 0
	pushedDeletes := 0
	if !syncPullOnly {
		localChanges, err := database.GetMemoriesSinceCursor(cursor, 0, true)
		if err != nil {
			return fmt.Errorf("reading local changes: %w", err)
		}
		localDeleted, err := database.GetDeletedSince(cursor)
		if err != nil {
			return fmt.Errorf("reading local tombstones: %w", err)
		}

		if len(localChanges) > 0 || len(localDeleted) > 0 {
			payload, err := json.Marshal(map[string]interface{}{
				"memories": localChanges,
				"deleted":  localDeleted,
			})
			if err != nil {
				return fmt.Errorf("encoding push payload: %w", err)
			}

			pushResp, err := syncHTTPDo(client, "POST", remote+"/api/sync/apply", token, payload)
			if err != nil {
				return err
			}
			defer pushResp.Body.Close()

			var pushResult struct {
				Applied int `json:"applied"`
				Skipped int `json:"skipped"`
				Deleted int `json:"deleted"`
			}
			if err := json.NewDecoder(pushResp.Body).Decode(&pushResult); err != nil {
				return fmt.Errorf("decoding push response: %w", err)
			}
			pushedCount = pushResult.Applied
			pushedDeletes = pushResult.Deleted
		}
	}

	// Advance cursor only after a complete successful pull. In push-only
	// mode there is no server_time; keep the existing cursor untouched so
	// the next pull still sees everything since the last full sync.
	newCursor := cursor
	if serverTimeStr != "" {
		serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
		if err != nil {
			return fmt.Errorf("parsing server_time: %w", err)
		}
		if err := database.SetSyncCursor(remote, serverTime); err != nil {
			return fmt.Errorf("saving sync cursor: %w", err)
		}
		newCursor = serverTime
	}

	fmt.Printf("pulled: %d applied / %d skipped / %d deleted, pushed: %d upserts / %d deletes, cursor: %s\n",
		appliedCount, skippedCount, deletedCount, pushedCount, pushedDeletes, newCursor.UTC().Format(time.RFC3339))
	return nil
}

// relayEnvelope is the plaintext structure encrypted into a relay blob. The
// relay server only ever sees the memory id, the operation timestamp and the
// ciphertext of this envelope.
type relayEnvelope struct {
	Op     string     `json:"op"` // "upsert" or "delete"
	Memory *db.Memory `json:"memory,omitempty"`
	ID     string     `json:"id"`
	TS     time.Time  `json:"ts"`
}

func runRelaySync(database *db.DB, remote, token, passphrase string) error {
	remote = strings.TrimRight(remote, "/")
	client := &http.Client{Timeout: 30 * time.Second}
	crypto := security.NewCryptoEngine()
	cursorKey := remote + "#relay"

	cursor, err := database.GetSyncCursor(cursorKey)
	if err != nil {
		return fmt.Errorf("reading sync cursor: %w", err)
	}

	applied, skipped, deleted := 0, 0, 0
	var serverTime time.Time

	if !syncPushOnly {
		pullURL := remote + "/api/sync/relay"
		if !cursor.IsZero() {
			pullURL += "?since=" + cursor.UTC().Format(time.RFC3339Nano)
		}
		pullResp, err := syncHTTPDo(client, "GET", pullURL, token, nil)
		if err != nil {
			return err
		}
		var pullResult struct {
			Blobs      []db.RelayBlob `json:"blobs"`
			ServerTime string         `json:"server_time"`
		}
		if err := json.NewDecoder(pullResp.Body).Decode(&pullResult); err != nil {
			pullResp.Body.Close()
			return fmt.Errorf("decoding relay response: %w", err)
		}
		pullResp.Body.Close()

		if pullResult.ServerTime != "" {
			serverTime, err = time.Parse(time.RFC3339Nano, pullResult.ServerTime)
			if err != nil {
				return fmt.Errorf("parsing server_time: %w", err)
			}
		}

		for _, b := range pullResult.Blobs {
			plaintext, err := crypto.Decrypt(b.Blob, passphrase)
			if err != nil {
				return fmt.Errorf("decrypting relay blob %s (wrong passphrase?): %w", b.ID, err)
			}
			var env relayEnvelope
			if err := json.Unmarshal(plaintext, &env); err != nil {
				return fmt.Errorf("decoding relay envelope %s: %w", b.ID, err)
			}
			switch env.Op {
			case "upsert":
				if env.Memory == nil || env.Memory.ID == "" {
					skipped++
					continue
				}
				ok, err := database.SyncUpsertMemoryIfNewer(env.Memory)
				if err != nil {
					return fmt.Errorf("applying memory %s: %w", env.Memory.ID, err)
				}
				if ok {
					applied++
				} else {
					skipped++
				}
			case "delete":
				removed, err := database.ApplyRemoteDelete(env.ID, env.TS)
				if err != nil {
					return fmt.Errorf("applying delete %s: %w", env.ID, err)
				}
				if removed {
					deleted++
				}
			default:
				skipped++
			}
		}
	}

	pushed := 0
	if !syncPullOnly {
		localChanges, err := database.GetMemoriesSinceCursor(cursor, 0, true)
		if err != nil {
			return fmt.Errorf("reading local changes: %w", err)
		}
		localDeleted, err := database.GetDeletedSince(cursor)
		if err != nil {
			return fmt.Errorf("reading local tombstones: %w", err)
		}

		var blobs []db.RelayBlob
		for _, m := range localChanges {
			env := relayEnvelope{Op: "upsert", Memory: m, ID: m.ID, TS: m.UpdatedAt}
			blob, err := encryptEnvelope(crypto, env, passphrase)
			if err != nil {
				return err
			}
			blobs = append(blobs, db.RelayBlob{ID: m.ID, UpdatedAt: m.UpdatedAt, Blob: blob})
		}
		for _, d := range localDeleted {
			env := relayEnvelope{Op: "delete", ID: d.ID, TS: d.DeletedAt}
			blob, err := encryptEnvelope(crypto, env, passphrase)
			if err != nil {
				return err
			}
			blobs = append(blobs, db.RelayBlob{ID: d.ID, UpdatedAt: d.DeletedAt, Blob: blob})
		}

		if len(blobs) > 0 {
			payload, err := json.Marshal(map[string]interface{}{"blobs": blobs})
			if err != nil {
				return fmt.Errorf("encoding relay payload: %w", err)
			}
			pushResp, err := syncHTTPDo(client, "POST", remote+"/api/sync/relay", token, payload)
			if err != nil {
				return err
			}
			var pushResult struct {
				Stored int `json:"stored"`
			}
			if err := json.NewDecoder(pushResp.Body).Decode(&pushResult); err != nil {
				pushResp.Body.Close()
				return fmt.Errorf("decoding relay push response: %w", err)
			}
			pushResp.Body.Close()
			pushed = pushResult.Stored
		}
	}

	if !serverTime.IsZero() {
		if err := database.SetSyncCursor(cursorKey, serverTime); err != nil {
			return fmt.Errorf("saving sync cursor: %w", err)
		}
	}

	fmt.Printf("relay pulled: %d applied / %d skipped / %d deleted, pushed: %d blobs, cursor: %s\n",
		applied, skipped, deleted, pushed, serverTime.UTC().Format(time.RFC3339Nano))
	return nil
}

func encryptEnvelope(crypto *security.CryptoEngine, env relayEnvelope, passphrase string) ([]byte, error) {
	plaintext, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("encoding relay envelope: %w", err)
	}
	blob, err := crypto.Encrypt(plaintext, passphrase)
	if err != nil {
		return nil, fmt.Errorf("encrypting relay envelope: %w", err)
	}
	return blob, nil
}
