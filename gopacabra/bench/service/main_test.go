package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type fakeItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

// fakeAPI — API gopacabra/stand/rest у пам'яті; broken псує відповідь однієї
// операції, щоб перевірити, що сценарій її помічає; "slow" — перший
// GET /items/{id} відповідає через 50 мс.
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
	slow := broken == "slow"
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		mu.Lock()
		if slow {
			slow = false
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
		}
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
			rep, err := scenario(t.Context(), srv.Client(), srv.URL, clients, n, seed, 0)
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
			if len(rep.lat) != n {
				t.Errorf("scenario latencies = %d, want one per request, %d", len(rep.lat), n)
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
	_, err := scenario(t.Context(), client, srv.URL, 1, 10, 5, 0)
	if err == nil || !strings.Contains(err.Error(), "empty database") {
		t.Errorf("scenario on a non-empty database = %v, want the empty-database error", err)
	}
}

// TestRecordRepeatable: запис сценарію з одним клієнтом однаковий побайтно для
// двох свіжих серверів і не містить заголовка Date (час відповіді).
func TestRecordRepeatable(t *testing.T) {
	record := func() string {
		srv := fakeAPI(t, "")
		var b strings.Builder
		client := srv.Client()
		client.Transport = &recorder{next: client.Transport, w: &b}
		if _, err := scenario(t.Context(), client, srv.URL, 1, 200, 20, 0); err != nil {
			t.Fatalf("scenario = %v", err)
		}
		return b.String()
	}
	first, second := record(), record()
	if first != second {
		t.Errorf("record differs between two runs:\n%s\n---\n%s", first, second)
	}
	for _, want := range []string{
		"GET /items?limit=1\n200\n",
		`POST /items {"name":"item-1","qty":1}` + "\n201\nContent-Length: 32\nContent-Type: ",
		`{"id":1,"name":"item-1","qty":1}` + "\n",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("record lacks %q", want)
		}
	}
	if strings.Contains(first, "Date:") {
		t.Error("record contains a Date header")
	}
}

// TestScenarioRate: з rate > 0 запит i суміші йде в момент i/rate від її
// початку, а затримка лічиться від цього моменту, а не від відправлення:
// повільна відповідь затримує наступні запити клієнта, і їхнє очікування
// входить у затримку (без coordinated omission). Годинник synctest —
// штучний, тож час точний.
func TestScenarioRate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const clients, n, seed, rate = 1, 20, 5, 100 // запит кожні 10 мс
		srv := fakeAPI(t, "slow")
		rep, err := scenario(t.Context(), srv.Client(), srv.URL, clients, n, seed, rate)
		if err != nil || len(rep.errs) > 0 {
			t.Fatalf("scenario = %v, errors %v", err, rep.errs)
		}
		if want := (n - 1) * time.Second / rate; rep.elapsed != want {
			t.Errorf("scenario elapsed = %v, want %v: the last request starts at (n-1)/rate", rep.elapsed, want)
		}
		// Перший GET /items/{id} триває 50 мс, тож чотири наступні запити
		// чекають свого часу на 40, 30, 20 і 10 мс довше.
		var sum, peak time.Duration
		for _, d := range rep.lat {
			sum += d
			peak = max(peak, d)
		}
		if peak != 50*time.Millisecond || sum != 150*time.Millisecond {
			t.Errorf("scenario latencies %v: max %v, sum %v, want 50ms and 150ms", rep.lat, peak, sum)
		}
	})
}

func TestPercentile(t *testing.T) {
	sorted := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, tc := range []struct {
		p    float64
		want int
	}{
		{0.5, 5},
		{0.99, 10},
		{0.1, 1},
		{1, 10},
	} {
		if got := percentile(sorted, tc.p); got != tc.want {
			t.Errorf("percentile(1..10, %v) = %d, want %d", tc.p, got, tc.want)
		}
	}
	if got := percentile([]int(nil), 0.5); got != 0 {
		t.Errorf("percentile(nil, 0.5) = %d, want 0", got)
	}
}

// TestReadProc: RSS і пікова RSS з status у КБ, CPU user і sys зі stat у
// тактах по 10 мс; ім'я процесу в stat може містити пробіли й дужки.
func TestReadProc(t *testing.T) {
	dir := t.TempDir()
	status := "Name:\trest\nVmPeak:\t  900000 kB\nVmHWM:\t  123456 kB\nVmRSS:\t   65432 kB\nThreads:\t9\n"
	stat := "4242 (a) b (c) S 1 4242 4242 0 -1 4194560 100 0 0 0 250 37 0 0 20 0 9 0 100 0 0\n"
	for name, text := range map[string]string{"status": status, "stat": stat} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readProc(dir)
	want := proc{rssKiB: 65432, hwmKiB: 123456, user: 2500 * time.Millisecond, sys: 370 * time.Millisecond}
	if err != nil || got != want {
		t.Errorf("readProc(%q) = %+v, %v, want %+v", dir, got, err, want)
	}
	if _, err := readProc(filepath.Join(dir, "missing")); err == nil {
		t.Error("readProc(missing dir) = nil error, want an error")
	}
}
