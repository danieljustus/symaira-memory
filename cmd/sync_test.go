package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
)

func helperTestDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "symmemory-sync-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	database, err := db.Open(config.Defaults())
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func fakeRemoteServer(t *testing.T, memories []*db.Memory, authRequired bool) *httptest.Server {
	t.Helper()
	serverTime := time.Now().UTC()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authRequired {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer valid-token" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		switch r.URL.Path {
		case "/api/sync/changes":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"memories":    memories,
				"server_time": serverTime.Format(time.RFC3339),
			})
		case "/api/sync/apply":
			var body struct {
				Memories []*db.Memory `json:"memories"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{
				"applied": len(body.Memories),
				"skipped": 0,
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestSyncFirstSyncNoCursor(t *testing.T) {
	localDB := helperTestDB(t)

	remoteMemories := []*db.Memory{
		{
			ID:        "remote-1",
			Content:   "Remote memory 1",
			Scope:     "global",
			Metadata:  map[string]string{},
			Embedding: []float32{0.1},
			UpdatedAt: time.Now().UTC(),
			CreatedAt: time.Now().UTC(),
		},
		{
			ID:        "remote-2",
			Content:   "Remote memory 2",
			Scope:     "global",
			Metadata:  map[string]string{},
			Embedding: []float32{0.2},
			UpdatedAt: time.Now().UTC(),
			CreatedAt: time.Now().UTC(),
		},
	}

	ts := fakeRemoteServer(t, remoteMemories, false)
	defer ts.Close()

	err := runSync(localDB, ts.URL, "")
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	m1, _ := localDB.GetMemory("remote-1")
	if m1 == nil {
		t.Error("expected remote-1 to be synced locally")
	}
	m2, _ := localDB.GetMemory("remote-2")
	if m2 == nil {
		t.Error("expected remote-2 to be synced locally")
	}

	cursor, _ := localDB.GetSyncCursor(ts.URL)
	if cursor.IsZero() {
		t.Error("expected cursor to be set after sync")
	}
}

func TestSyncIncremental(t *testing.T) {
	localDB := helperTestDB(t)

	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	newTime := time.Now().UTC()

	oldMemory := &db.Memory{
		ID:        "old-mem",
		Content:   "Old memory",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
		UpdatedAt: oldTime,
		CreatedAt: oldTime,
	}

	localDB.SaveMemory(oldMemory)
	localDB.SetSyncCursor("http://fake-remote", oldTime)

	newMemory := &db.Memory{
		ID:        "new-mem",
		Content:   "New memory",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.2},
		UpdatedAt: newTime,
		CreatedAt: newTime,
	}

	ts := fakeRemoteServer(t, []*db.Memory{newMemory}, false)
	defer ts.Close()

	err := runSync(localDB, ts.URL, "")
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	m, _ := localDB.GetMemory("new-mem")
	if m == nil {
		t.Error("expected new-mem to be synced")
	}
}

func TestSyncLWWSkip(t *testing.T) {
	localDB := helperTestDB(t)

	existingTime := time.Now().UTC()
	existing := &db.Memory{
		ID:        "lww-test",
		Content:   "Local version",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
		UpdatedAt: existingTime,
		CreatedAt: existingTime,
	}
	localDB.SaveMemory(existing)

	olderRemote := &db.Memory{
		ID:        "lww-test",
		Content:   "Older remote version",
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
		UpdatedAt: existingTime.Add(-1 * time.Hour),
		CreatedAt: existingTime.Add(-1 * time.Hour),
	}

	ts := fakeRemoteServer(t, []*db.Memory{olderRemote}, false)
	defer ts.Close()

	err := runSync(localDB, ts.URL, "")
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	m, _ := localDB.GetMemory("lww-test")
	if m.Content != "Local version" {
		t.Errorf("expected LWW to skip older remote, got content: %q", m.Content)
	}
}

func TestSync401Handling(t *testing.T) {
	localDB := helperTestDB(t)

	ts := fakeRemoteServer(t, []*db.Memory{}, true)
	defer ts.Close()

	err := runSync(localDB, ts.URL, "wrong-token")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention 401, got: %v", err)
	}
}

func paginatedFakeServer(t *testing.T, allMemories []*db.Memory, pageSize int) *httptest.Server {
	t.Helper()
	serverTime := time.Now().UTC()

	sort.Slice(allMemories, func(i, j int) bool {
		return allMemories[i].UpdatedAt.Before(allMemories[j].UpdatedAt)
	})

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sync/changes":
			var since time.Time
			if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
				decoded, err := base64.StdEncoding.DecodeString(cursorStr)
				if err != nil {
					http.Error(w, "bad cursor", http.StatusBadRequest)
					return
				}
				parsed, err := time.Parse(time.RFC3339Nano, string(decoded))
				if err != nil {
					http.Error(w, "bad cursor format", http.StatusBadRequest)
					return
				}
				since = parsed
			} else if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
				parsed, err := time.Parse(time.RFC3339, sinceStr)
				if err != nil {
					http.Error(w, "bad since", http.StatusBadRequest)
					return
				}
				since = parsed
			}

			var page []*db.Memory
			for _, m := range allMemories {
				if m.UpdatedAt.After(since) {
					page = append(page, m)
				}
			}

			var nextCursor string
			if len(page) > pageSize {
				page = page[:pageSize]
				last := page[len(page)-1]
				nextCursor = base64.StdEncoding.EncodeToString([]byte(last.UpdatedAt.Format(time.RFC3339Nano)))
			}

			resp := map[string]interface{}{
				"memories":    page,
				"server_time": serverTime.Format(time.RFC3339),
			}
			if nextCursor != "" {
				resp["next_cursor"] = nextCursor
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/api/sync/apply":
			var body struct {
				Memories []*db.Memory `json:"memories"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{
				"applied": len(body.Memories),
				"skipped": 0,
			})

		default:
			http.NotFound(w, r)
		}
	}))
}

func TestSyncMultiPage(t *testing.T) {
	localDB := helperTestDB(t)

	base := time.Now().UTC()
	memories := make([]*db.Memory, 5)
	for i := range memories {
		memories[i] = &db.Memory{
			ID:        "mem-" + string(rune('a'+i)),
			Content:   "Memory " + string(rune('A'+i)),
			Scope:     "global",
			Metadata:  map[string]string{},
			Embedding: []float32{float32(i) * 0.1},
			UpdatedAt: base.Add(time.Duration(i) * time.Minute),
			CreatedAt: base,
		}
	}

	ts := paginatedFakeServer(t, memories, 2)
	defer ts.Close()

	err := runSync(localDB, ts.URL, "")
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	for _, m := range memories {
		got, _ := localDB.GetMemory(m.ID)
		if got == nil {
			t.Errorf("expected memory %q to be synced", m.ID)
		}
	}

	cursor, _ := localDB.GetSyncCursor(ts.URL)
	if cursor.IsZero() {
		t.Error("expected cursor to be set after multi-page sync")
	}
}

func TestSyncSecondSyncNoLoss(t *testing.T) {
	localDB := helperTestDB(t)

	base := time.Now().UTC()
	memories := make([]*db.Memory, 5)
	for i := range memories {
		memories[i] = &db.Memory{
			ID:        "nl-" + string(rune('a'+i)),
			Content:   "NoLoss " + string(rune('A'+i)),
			Scope:     "global",
			Metadata:  map[string]string{},
			Embedding: []float32{float32(i) * 0.1},
			UpdatedAt: base.Add(time.Duration(i) * time.Minute),
			CreatedAt: base,
		}
	}

	ts := paginatedFakeServer(t, memories, 2)
	defer ts.Close()

	err := runSync(localDB, ts.URL, "")
	if err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	err = runSync(localDB, ts.URL, "")
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}

	for _, m := range memories {
		got, _ := localDB.GetMemory(m.ID)
		if got == nil {
			t.Errorf("memory %q missing after second sync", m.ID)
		}
	}
}
