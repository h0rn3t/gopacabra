package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type fakeItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

// fakeAPI — API owngo/stand/rest у пам'яті; broken псує відповідь однієї
// операції, щоб перевірити, що сценарій її помічає.
func fakeAPI(t *testing.T, broken string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	items := map[int]fakeItem{}
	next := 1
	write := func(w http.ResponseWriter, code int, v any) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Error(err)
		}
		w.WriteHeader(code)
		_, _ = w.Write(b) // відповідь тесту; клієнт сам звітує про обрив
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items", func(w http.ResponseWriter, r *http.Request) {
		after, _ := strconv.Atoi(r.URL.Query().Get("after")) // відсутній — 0
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			limit = 100
		}
		mu.Lock()
		defer mu.Unlock()
		list := []fakeItem{}
		for id := after + 1; id < next && len(list) < limit; id++ {
			if it, ok := items[id]; ok {
				list = append(list, it)
			}
		}
		if broken == "list" && after > 0 {
			list = list[1:]
		}
		write(w, 200, list)
	})
	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		var it fakeItem
		if err := json.NewDecoder(r.Body).Decode(&it); err != nil {
			t.Error(err)
		}
		mu.Lock()
		defer mu.Unlock()
		it.ID = next
		next++
		items[it.ID] = it
		write(w, 201, it)
	})
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		mu.Lock()
		it, ok := items[id]
		mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		if broken == "get" {
			it.Qty++
		}
		write(w, 200, it)
	})
	mux.HandleFunc("PUT /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		var it fakeItem
		if err := json.NewDecoder(r.Body).Decode(&it); err != nil {
			t.Error(err)
		}
		it.ID = id
		mu.Lock()
		items[id] = it
		mu.Unlock()
		write(w, 200, it)
	})
	mux.HandleFunc("DELETE /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		mu.Lock()
		defer mu.Unlock()
		if _, ok := items[id]; !ok || broken == "delete" {
			http.NotFound(w, r)
			return
		}
		delete(items, id)
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewTestServer(t, mux)
}

func TestScenario(t *testing.T) {
	const clients, n, seed = 4, 2000, 50
	cases := []struct {
		broken  string
		wantErr bool
	}{
		{"", false},
		{"get", true},
		{"list", true},
		{"delete", true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("break %q", tc.broken), func(t *testing.T) {
			srv := fakeAPI(t, tc.broken)
			rep, err := scenario(t.Context(), srv.Client(), srv.URL, clients, n, seed)
			if err != nil {
				t.Fatalf("scenario = %v, want the seed to pass", err)
			}
			total := 0
			for _, k := range rep.ops {
				total += k
			}
			if total != n {
				t.Errorf("scenario ops = %v, sum %d, want %d", rep.ops, total, n)
			}
			// Частки суміші — з допуском: 2000 запитів, ±3 процентні пункти.
			for o, want := range map[op]int{opGet: 60, opList: 20, opUpdate: 5} {
				if got := 100 * rep.ops[o] / n; got < want-3 || got > want+3 {
					t.Errorf("scenario op %d = %d%%, want %d%%", o, got, want)
				}
			}
			if got := 100 * (rep.ops[opCreate] + rep.ops[opDelete]) / n; got < 12 || got > 18 {
				t.Errorf("scenario create+delete = %d%%, want 15%%", got)
			}
			if gotErr := len(rep.errs) > 0; gotErr != tc.wantErr {
				t.Errorf("scenario errors = %d (%v), want errors %v", len(rep.errs), rep.errs, tc.wantErr)
			}
		})
	}
}

func TestScenarioNeedsEmptyDatabase(t *testing.T) {
	srv := fakeAPI(t, "")
	client := srv.Client()
	if code, _, err := call(t.Context(), client, "POST", srv.URL+"/items", itemBody(1)); err != nil || code != 201 {
		t.Fatalf("POST /items = %d %v, want 201", code, err)
	}
	_, err := scenario(t.Context(), client, srv.URL, 1, 10, 5)
	if err == nil || !strings.Contains(err.Error(), "empty database") {
		t.Errorf("scenario on a non-empty database = %v, want the empty-database error", err)
	}
}
