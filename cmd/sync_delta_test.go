package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/danieljustus/symaira-memory/internal/db"
)

func resetSyncFlags(t *testing.T) {
	t.Helper()
	oldPush, oldPull := syncPushOnly, syncPullOnly
	syncPushOnly, syncPullOnly = false, false
	t.Cleanup(func() { syncPushOnly, syncPullOnly = oldPush, oldPull })
}

func helperMemory(id, content string, updatedAt time.Time) *db.Memory {
	return &db.Memory{
		ID:        id,
		Content:   content,
		Scope:     "global",
		Metadata:  map[string]string{},
		Embedding: []float32{0.1},
		CreatedAt: updatedAt,
		UpdatedAt: updatedAt,
	}
}

// deltaServer simulates the /api/sync/changes + /api/sync/apply endpoints
// with tombstone support, recording what was pushed.
type deltaServer struct {
	mu       sync.Mutex
	memories []*db.Memory
	deleted  []db.DeletedMemory
	pushed   struct {
		Memories []*db.Memory       `json:"memories"`
		Deleted  []db.DeletedMemory `json:"deleted"`
	}
}

func (ds *deltaServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ds.mu.Lock()
		defer ds.mu.Unlock()
		switch r.URL.Path {
		case "/api/sync/changes":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"memories":    ds.memories,
				"deleted":     ds.deleted,
				"server_time": time.Now().UTC().Format(time.RFC3339),
			})
		case "/api/sync/apply":
			if err := json.NewDecoder(r.Body).Decode(&ds.pushed); err != nil {
				t.Errorf("decoding push: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{
				"applied": len(ds.pushed.Memories),
				"deleted": len(ds.pushed.Deleted),
			})
		default:
			http.NotFound(w, r)
		}
	}
}

func TestSyncDeletePropagationPull(t *testing.T) {
	resetSyncFlags(t)
	localDB := helperTestDB(t)

	m := helperMemory("55555555-5555-5555-5555-555555555555", "to be deleted remotely", time.Now().UTC().Add(-2*time.Hour))
	if err := localDB.SaveMemory(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	ds := &deltaServer{deleted: []db.DeletedMemory{{ID: m.ID, DeletedAt: time.Now().UTC()}}}
	ts := httptest.NewServer(ds.handler(t))
	defer ts.Close()

	if err := runSync(localDB, ts.URL, ""); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	got, err := localDB.GetMemory(m.ID)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got != nil {
		t.Fatal("remote delete must remove the local memory")
	}
}

func TestSyncDeletePropagationPush(t *testing.T) {
	resetSyncFlags(t)
	localDB := helperTestDB(t)

	m := helperMemory("66666666-6666-6666-6666-666666666666", "local, then deleted", time.Now().UTC())
	if err := localDB.SaveMemory(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := localDB.DeleteMemory(m.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	ds := &deltaServer{}
	ts := httptest.NewServer(ds.handler(t))
	defer ts.Close()

	if err := runSync(localDB, ts.URL, ""); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	found := false
	for _, d := range ds.pushed.Deleted {
		if d.ID == m.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("local delete must be pushed as tombstone, pushed=%+v", ds.pushed.Deleted)
	}
}

func TestSyncPullOnlyDoesNotPush(t *testing.T) {
	resetSyncFlags(t)
	syncPullOnly = true
	localDB := helperTestDB(t)

	m := helperMemory("77777777-7777-7777-7777-777777777777", "local only", time.Now().UTC())
	if err := localDB.SaveMemory(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	ds := &deltaServer{}
	ts := httptest.NewServer(ds.handler(t))
	defer ts.Close()

	if err := runSync(localDB, ts.URL, ""); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	if len(ds.pushed.Memories) != 0 {
		t.Fatalf("pull-only must not push, pushed=%d memories", len(ds.pushed.Memories))
	}
}

func TestSyncPushOnlyDoesNotApplyRemote(t *testing.T) {
	resetSyncFlags(t)
	syncPushOnly = true
	localDB := helperTestDB(t)

	remoteMem := helperMemory("88888888-8888-8888-8888-888888888888", "remote only", time.Now().UTC())
	ds := &deltaServer{memories: []*db.Memory{remoteMem}}
	ts := httptest.NewServer(ds.handler(t))
	defer ts.Close()

	if err := runSync(localDB, ts.URL, ""); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	got, err := localDB.GetMemory(remoteMem.ID)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got != nil {
		t.Fatal("push-only must not apply remote changes")
	}
}

// relayServer stores opaque blobs exactly like the real relay endpoint and
// lets the test inspect everything it ever saw.
type relayServer struct {
	mu    sync.Mutex
	blobs map[string]db.RelayBlob
	raw   [][]byte // every request body, for plaintext-leak checks
}

func (rs *relayServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		if r.URL.Path != "/api/sync/relay" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			var out []db.RelayBlob
			for _, b := range rs.blobs {
				out = append(out, b)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"blobs":       out,
				"server_time": time.Now().UTC().Format(time.RFC3339Nano),
			})
		case http.MethodPost:
			var body struct {
				Blobs []db.RelayBlob `json:"blobs"`
			}
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(r.Body); err != nil {
				t.Errorf("reading relay body: %v", err)
			}
			rs.raw = append(rs.raw, buf.Bytes())
			if err := json.Unmarshal(buf.Bytes(), &body); err != nil {
				t.Errorf("decoding relay push: %v", err)
			}
			stored := 0
			for _, b := range body.Blobs {
				existing, ok := rs.blobs[b.ID]
				if !ok || b.UpdatedAt.After(existing.UpdatedAt) {
					rs.blobs[b.ID] = b
					stored++
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{"stored": stored})
		}
	}
}

func TestRelaySyncConvergesAndLeaksNoPlaintext(t *testing.T) {
	resetSyncFlags(t)
	const passphrase = "test-relay-passphrase"
	const secret = "the secret memory content nobody may see"

	rs := &relayServer{blobs: map[string]db.RelayBlob{}}
	ts := httptest.NewServer(rs.handler(t))
	defer ts.Close()

	// Device A: create a memory and push it through the relay.
	dbA := helperTestDB(t)
	m := helperMemory("99999999-9999-9999-9999-999999999999", secret, time.Now().UTC())
	if err := dbA.SaveMemory(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := runRelaySync(dbA, ts.URL, "", passphrase); err != nil {
		t.Fatalf("relay sync A failed: %v", err)
	}

	// The relay must never see the plaintext content.
	rs.mu.Lock()
	for _, raw := range rs.raw {
		if bytes.Contains(raw, []byte(secret)) {
			rs.mu.Unlock()
			t.Fatal("relay request contained plaintext memory content")
		}
	}
	for _, b := range rs.blobs {
		if bytes.Contains(b.Blob, []byte(secret)) {
			rs.mu.Unlock()
			t.Fatal("relay-stored blob contained plaintext memory content")
		}
	}
	rs.mu.Unlock()

	// Device B: pull through the relay and converge.
	dbB := helperTestDB(t)
	if err := runRelaySync(dbB, ts.URL, "", passphrase); err != nil {
		t.Fatalf("relay sync B failed: %v", err)
	}
	got, err := dbB.GetMemory(m.ID)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got == nil || got.Content != secret {
		t.Fatalf("device B must converge to the plaintext memory, got %+v", got)
	}

	// Wrong passphrase must fail loudly, not apply garbage.
	dbC := helperTestDB(t)
	if err := runRelaySync(dbC, ts.URL, "", "wrong-passphrase"); err == nil {
		t.Fatal("relay sync with wrong passphrase must fail")
	}
}

func TestRelaySyncPropagatesDeletes(t *testing.T) {
	resetSyncFlags(t)
	const passphrase = "test-relay-passphrase"

	rs := &relayServer{blobs: map[string]db.RelayBlob{}}
	ts := httptest.NewServer(rs.handler(t))
	defer ts.Close()

	// Device A creates and pushes.
	dbA := helperTestDB(t)
	m := helperMemory("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "shared fact", time.Now().UTC().Add(-1*time.Minute))
	if err := dbA.SaveMemory(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := runRelaySync(dbA, ts.URL, "", passphrase); err != nil {
		t.Fatalf("relay sync A failed: %v", err)
	}

	// Device B pulls the memory.
	dbB := helperTestDB(t)
	if err := runRelaySync(dbB, ts.URL, "", passphrase); err != nil {
		t.Fatalf("relay sync B failed: %v", err)
	}
	if got, _ := dbB.GetMemory(m.ID); got == nil {
		t.Fatal("device B must have the memory before the delete")
	}

	// Device A deletes and pushes the tombstone.
	if err := dbA.DeleteMemory(m.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if err := runRelaySync(dbA, ts.URL, "", passphrase); err != nil {
		t.Fatalf("relay sync A (delete) failed: %v", err)
	}

	// Device B pulls again; the memory must be gone.
	if err := runRelaySync(dbB, ts.URL, "", passphrase); err != nil {
		t.Fatalf("relay sync B (delete) failed: %v", err)
	}
	if got, _ := dbB.GetMemory(m.ID); got != nil {
		t.Fatal("delete must propagate through the relay")
	}
}

func TestReadPassphraseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pass")
	if err := os.WriteFile(path, []byte("  secret-pass \n"), 0600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	got, err := readPassphraseFile(path)
	if err != nil {
		t.Fatalf("readPassphraseFile failed: %v", err)
	}
	if got != "secret-pass" {
		t.Fatalf("expected trimmed passphrase, got %q", got)
	}

	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, []byte("\n"), 0600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := readPassphraseFile(empty); err == nil {
		t.Fatal("empty passphrase file must error")
	}
}
