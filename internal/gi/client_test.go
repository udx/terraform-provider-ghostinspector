package gi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(server *httptest.Server) *Client {
	c := NewClient("test-key")
	c.baseURL = server.URL
	c.httpClient = &http.Client{Timeout: 5 * time.Second}
	return c
}

func TestSanitize(t *testing.T) {
	in := []byte("{\"a\":\"line1\nline2\ttabbed\"}\x00\x07\x0b\x0d\x1f")
	out := sanitize(in)
	s := string(out)
	if strings.Contains(s, "\x00") || strings.Contains(s, "\x0d") || strings.Contains(s, "\x1f") {
		t.Fatalf("control chars not stripped: %q", s)
	}
	if !strings.Contains(s, "\n") || !strings.Contains(s, "\t") {
		t.Fatalf("tab/newline must be preserved: %q", s)
	}
}

func TestEnvelopeDecode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"SUCCESS","data":{"_id":"abc123","name":"x"}}`))
	}))
	defer server.Close()

	data, err := testClient(server).do(context.Background(), http.MethodGet, "/thing/", nil)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]string
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["_id"] != "abc123" {
		t.Fatalf("unexpected data: %s", data)
	}
}

func TestBareDocumentDecode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// test export style response: bare doc, with a raw control char inside
		w.Write([]byte("{\"name\":\"t\",\"steps\":[{\"command\":\"open\",\"value\":\"a\x01b\"}]}"))
	}))
	defer server.Close()

	data, err := testClient(server).do(context.Background(), http.MethodGet, "/export/", nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["name"] != "t" {
		t.Fatalf("unexpected doc: %s", data)
	}
}

func TestErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"ERROR","message":"Test not found"}`))
	}))
	defer server.Close()

	_, err := testClient(server).do(context.Background(), http.MethodGet, "/tests/gone/", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !NotFound(err) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestRetryOn429(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"code":"SUCCESS","data":{"ok":true}}`))
	}))
	defer server.Close()

	c := testClient(server)
	start := time.Now()
	data, err := c.do(context.Background(), http.MethodGet, "/x/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
	if time.Since(start) < 2*time.Second {
		t.Fatalf("expected backoff between attempts")
	}
	var obj map[string]bool
	if err := json.Unmarshal(data, &obj); err != nil || !obj["ok"] {
		t.Fatalf("unexpected data: %s %v", data, err)
	}
}

func TestAPIKeyAppended(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"code":"SUCCESS","data":{}}`))
	}))
	defer server.Close()

	if _, err := testClient(server).do(context.Background(), http.MethodGet, "/x/?count=5", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "apiKey=test-key") || !strings.Contains(gotQuery, "count=5") {
		t.Fatalf("query mangled: %s", gotQuery)
	}
}
