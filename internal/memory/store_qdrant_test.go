package memory

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const qdrantTestDims = 4

func qdrantTestModel() ModelID {
	return ModelID{Name: "test-embed", Dims: qdrantTestDims}
}

func qdrantInfoBody(size int) string {
	return `{"result":{"config":{"params":{"vectors":{"size":` +
		strconv.Itoa(size) + `,"distance":"Cosine"}}}},"status":"ok"}`
}

func qdrantTestStore(t *testing.T, handler http.HandlerFunc) *QdrantStore {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/collections/wiki" {
			io.WriteString(w, qdrantInfoBody(qdrantTestDims))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	store, err := OpenQdrantStore(srv.URL, "wiki", qdrantTestModel())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

func TestQdrantOpenCreatesMissingCollection(t *testing.T) {
	var created []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"status":{"error":"Not found: Collection doesn't exist!"}}`)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/collections/wiki" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		created, _ = io.ReadAll(r.Body)
		io.WriteString(w, `{"result":true,"status":"ok"}`)
	}))
	defer srv.Close()

	if _, err := OpenQdrantStore(srv.URL, "wiki", qdrantTestModel()); err != nil {
		t.Fatalf("open store: %v", err)
	}
	var body struct {
		Vectors qdrantVectorParams `json:"vectors"`
	}
	if err := json.Unmarshal(created, &body); err != nil {
		t.Fatalf("decode create body %q: %v", created, err)
	}
	if body.Vectors.Size != qdrantTestDims || body.Vectors.Distance != "Cosine" {
		t.Fatalf("created %+v, want size %d distance Cosine", body.Vectors, qdrantTestDims)
	}
}

func TestQdrantOpenRejectsDimensionMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("collection must not be touched on mismatch, got %s", r.Method)
		}
		io.WriteString(w, qdrantInfoBody(qdrantTestDims+1))
	}))
	defer srv.Close()

	_, err := OpenQdrantStore(srv.URL, "wiki", qdrantTestModel())
	if err == nil {
		t.Fatal("want an error when the collection holds another model's vectors")
	}
}

func TestQdrantOpenReportsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"status":{"error":"storage is on fire"}}`)
	}))
	defer srv.Close()

	_, err := OpenQdrantStore(srv.URL, "wiki", qdrantTestModel())
	if err == nil {
		t.Fatal("want an error on a 500")
	}
	if got := err.Error(); !strings.Contains(got, "500") || !strings.Contains(got, "storage is on fire") {
		t.Fatalf("error %q must carry the status and a body excerpt", got)
	}
}

func TestQdrantUpsertBodyShape(t *testing.T) {
	var got struct {
		Points []qdrantPoint `json:"points"`
	}
	var query string
	store := qdrantTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/collections/wiki/points" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		query = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode upsert body: %v", err)
		}
		io.WriteString(w, `{"result":{"status":"completed"},"status":"ok"}`)
	})

	entry := Entry{Key: "tools/x.md#42", Path: "tools/x.md", Heading: "h", Line: 42, Hash: "abc",
		Vector: Vector{1, 0, 0, 0}}
	if err := store.Upsert([]Entry{entry}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if query != "wait=true" {
		t.Fatalf("query %q, want wait=true", query)
	}
	if len(got.Points) != 1 {
		t.Fatalf("sent %d points, want 1", len(got.Points))
	}
	point := got.Points[0]
	if point.ID != qdrantPointID(entry.Key) || point.ID == 0 {
		t.Fatalf("point id %d, want the derived id %d", point.ID, qdrantPointID(entry.Key))
	}
	if point.Payload.Key != entry.Key || point.Payload.Path != entry.Path ||
		point.Payload.Heading != entry.Heading || point.Payload.Line != entry.Line ||
		point.Payload.Hash != entry.Hash {
		t.Fatalf("payload %+v does not carry the entry", point.Payload)
	}
	if len(point.Vector) != qdrantTestDims {
		t.Fatalf("vector has %d dims, want %d", len(point.Vector), qdrantTestDims)
	}
}

func TestQdrantUpsertRejectsWrongDims(t *testing.T) {
	store := qdrantTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a mismatched vector must never reach the server")
	})
	err := store.Upsert([]Entry{{Key: "a.md#1", Vector: Vector{1, 2}}})
	if err == nil {
		t.Fatal("want an error for a vector of the wrong width")
	}
}

func TestQdrantNearestSortsByScore(t *testing.T) {
	store := qdrantTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/wiki/points/query" {
			t.Errorf("nearest hit %s, want /points/query", r.URL.Path)
		}
		io.WriteString(w, `{"result":{"points":[
			{"score":0.5,"payload":{"key":"b.md#1","path":"b.md","line":1}},
			{"score":0.9,"payload":{"key":"a.md#7","path":"a.md","line":7}},
			{"score":0.5,"payload":{"key":"a.md#2","path":"a.md","line":2}}]},"status":"ok"}`)
	})

	got, _ := store.Nearest(Vector{1, 0, 0, 0}, 3)
	want := []string{"a.md#7", "a.md#2", "b.md#1"}
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for i, key := range want {
		if got[i].Key != key {
			t.Fatalf("result %d is %q, want %q (order %+v)", i, got[i].Key, key, got)
		}
	}
	if got[0].Score != 0.9 || got[0].Path != "a.md" || got[0].Line != 7 {
		t.Fatalf("top result %+v lost its payload", got[0])
	}
}

func TestQdrantHashesScrollsEveryPage(t *testing.T) {
	page := 0
	store := qdrantTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/wiki/points/scroll" {
			t.Errorf("hashes hit %s, want /points/scroll", r.URL.Path)
		}
		page++
		if page == 1 {
			io.WriteString(w, `{"result":{"points":[{"payload":{"key":"a.md#1","hash":"h1"}}],
				"next_page_offset":12},"status":"ok"}`)
			return
		}
		io.WriteString(w, `{"result":{"points":[{"payload":{"key":"b.md#2","hash":"h2"}}],
			"next_page_offset":null},"status":"ok"}`)
	})

	got := store.Hashes()
	if len(got) != 2 || got["a.md#1"] != "h1" || got["b.md#2"] != "h2" {
		t.Fatalf("hashes %v, want both pages merged", got)
	}
}

func TestQdrantDeletePathsSendsFilter(t *testing.T) {
	var got struct {
		Filter struct {
			Must []struct {
				Key   string `json:"key"`
				Match struct {
					Any []string `json:"any"`
				} `json:"match"`
			} `json:"must"`
		} `json:"filter"`
	}
	store := qdrantTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/wiki/points/delete" {
			t.Errorf("delete hit %s, want /points/delete", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode delete body: %v", err)
		}
		io.WriteString(w, `{"result":{"status":"completed"},"status":"ok"}`)
	})

	if err := store.DeletePaths([]string{"a.md", "b.md"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(got.Filter.Must) != 1 || got.Filter.Must[0].Key != "path" {
		t.Fatalf("filter %+v must match on the path payload field", got.Filter)
	}
	if len(got.Filter.Must[0].Match.Any) != 2 {
		t.Fatalf("filter matches %v, want both paths", got.Filter.Must[0].Match.Any)
	}
}

func TestQdrantDeletePathsSkipsEmpty(t *testing.T) {
	store := qdrantTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("an empty delete must not reach the server")
	})
	if err := store.DeletePaths(nil); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestQdrantLiveRoundTrip(t *testing.T) {
	base := os.Getenv("QDRANT_URL")
	if base == "" {
		t.Skip("set QDRANT_URL to run this against a real Qdrant instance")
	}
	name := fmt.Sprintf("jardin_test_%d", time.Now().UnixNano())
	store, err := OpenQdrantStore(base, name, qdrantTestModel())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { dropQdrantCollection(t, base, name) })

	entries := []Entry{
		{Key: "a.md#1", Path: "a.md", Heading: "one", Line: 1, Hash: "h1", Vector: Vector{1, 0, 0, 0}},
		{Key: "b.md#2", Path: "b.md", Heading: "two", Line: 2, Hash: "h2", Vector: Vector{0, 1, 0, 0}},
	}
	if err := store.Upsert(entries); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if hashes := store.Hashes(); len(hashes) != 2 || hashes["a.md#1"] != "h1" {
		t.Fatalf("hashes %v, want both entries", hashes)
	}
	near, _ := store.Nearest(Vector{1, 0, 0, 0}, 2)
	if len(near) != 2 || near[0].Key != "a.md#1" || near[0].Score <= near[1].Score {
		t.Fatalf("nearest %+v, want a.md#1 ranked first", near)
	}
	if err := store.DeletePaths([]string{"a.md"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if hashes := store.Hashes(); len(hashes) != 1 || hashes["b.md#2"] != "h2" {
		t.Fatalf("hashes %v after delete, want only b.md#2", hashes)
	}
	if _, err := OpenQdrantStore(base, name, ModelID{Name: "other", Dims: qdrantTestDims + 1}); err == nil {
		t.Fatal("want an error when reopening with a model of another width")
	}
}

func dropQdrantCollection(t *testing.T, base, name string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, base+"/collections/"+name, nil)
	if err != nil {
		t.Fatalf("build delete: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("drop collection %q: %v", name, err)
	}
	resp.Body.Close()
}
