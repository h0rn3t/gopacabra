package main

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPI — контракт REST API сервісу фази 11 (P11.4). Випадки йдуть по черзі
// над однією базою: кожен бачить стан, який лишили попередні.
func TestAPI(t *testing.T) {
	db, err := openDB(t.Context(), filepath.Join(t.TempDir(), "items.db"))
	if err != nil {
		t.Fatalf("openDB = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close = %v", err)
		}
	})
	app := newApp(db)

	cases := []struct {
		name, method, target, body string
		wantCode                   int
		wantBody                   string // "-" — тіло не перевіряється
	}{
		{"health", "GET", "/healthz", "", 200, "ok"},
		{"list empty", "GET", "/items", "", 200, "[]"},
		{"create a", "POST", "/items", `{"name":"a","qty":1}`, 201, `{"id":1,"name":"a","qty":1}`},
		{"create b", "POST", "/items", `{"name":"b","qty":2}`, 201, `{"id":2,"name":"b","qty":2}`},
		{"create c", "POST", "/items", `{"name":"c","qty":3}`, 201, `{"id":3,"name":"c","qty":3}`},
		{"create invalid json", "POST", "/items", `{"name":`, 400, "-"},
		{"create empty name", "POST", "/items", `{"name":"","qty":1}`, 400, "-"},
		{"create negative qty", "POST", "/items", `{"name":"d","qty":-1}`, 400, "-"},
		{"get", "GET", "/items/2", "", 200, `{"id":2,"name":"b","qty":2}`},
		{"get missing", "GET", "/items/999", "", 404, "-"},
		{"get bad id", "GET", "/items/abc", "", 400, "-"},
		{"update", "PUT", "/items/2", `{"name":"b2","qty":5}`, 200, `{"id":2,"name":"b2","qty":5}`},
		{"get updated", "GET", "/items/2", "", 200, `{"id":2,"name":"b2","qty":5}`},
		{"update missing", "PUT", "/items/999", `{"name":"x","qty":1}`, 404, "-"},
		{"update invalid json", "PUT", "/items/2", `{`, 400, "-"},
		{"update empty name", "PUT", "/items/2", `{"name":"","qty":1}`, 400, "-"},
		{"update negative qty", "PUT", "/items/2", `{"name":"x","qty":-1}`, 400, "-"},
		{"update bad id", "PUT", "/items/abc", `{"name":"x","qty":1}`, 400, "-"},
		{"delete", "DELETE", "/items/3", "", 204, ""},
		{"delete again", "DELETE", "/items/3", "", 404, "-"},
		{"get deleted", "GET", "/items/3", "", 404, "-"},
		{"delete bad id", "DELETE", "/items/abc", "", 400, "-"},
		{"list", "GET", "/items", "", 200, `[{"id":1,"name":"a","qty":1},{"id":2,"name":"b2","qty":5}]`},
		{"list first page", "GET", "/items?limit=1", "", 200, `[{"id":1,"name":"a","qty":1}]`},
		{"list after", "GET", "/items?after=1", "", 200, `[{"id":2,"name":"b2","qty":5}]`},
		{"list past end", "GET", "/items?after=2", "", 200, "[]"},
		{"list max limit", "GET", "/items?limit=1000", "", 200, "-"},
		{"list zero limit", "GET", "/items?limit=0", "", 400, "-"},
		{"list limit over max", "GET", "/items?limit=1001", "", 400, "-"},
		{"list bad after", "GET", "/items?after=abc", "", 400, "-"},
		{"unknown route", "GET", "/nope", "", 404, "-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), tc.method, tc.target, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.target, err)
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.TrimSpace(string(b))
			if resp.StatusCode != tc.wantCode || tc.wantBody != "-" && body != tc.wantBody {
				t.Errorf("%s %s = %d %q, want %d %q", tc.method, tc.target, resp.StatusCode, body, tc.wantCode, tc.wantBody)
			}
		})
	}
}
