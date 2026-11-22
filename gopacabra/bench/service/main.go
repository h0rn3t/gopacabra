// Command service runs the load scenario of the Gopacabra phase 11 service
// (gopacabra/stand/rest, P11.4). It seeds items 1..seed into an empty database,
// then c clients send n requests in a fixed mix and every response is checked
// against what the API promises:
//
//	60% GET /items/{id}               seeded id, 200 with the seeded item
//	20% GET /items?after={a}&limit=20 a below seed, 200 starting at id a+1
//	10% POST /items                   201 with a new id, kept by the client
//	 5% PUT /items/{id}               seeded id, same values, 200
//	 5% DELETE /items/{id}            an id the client created, 204;
//	                                  a POST while the client has none
//
// Seeded items are never changed or deleted, so each expected answer is
// known whatever the order of requests. The command exits non-zero on any
// unexpected answer.
//
// -record FILE writes every request and its response (status, headers but
// Date, body). With -c 1 the record is the same byte for byte on every run, so
// the records of two builds of the service compare with cmp.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	base := fs.String("url", "http://127.0.0.1:8080", "service base URL")
	clients := fs.Int("c", 64, "concurrent clients")
	n := fs.Int("n", 100_000, "requests after seeding")
	seed := fs.Int("seed", 1000, "items to seed")
	record := fs.String("record", "", "write requests and responses to this file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *clients < 1 || *n < 0 || *seed < 1 {
		return errors.New("-c and -seed must be at least 1, -n not negative")
	}

	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = *clients
	client := &http.Client{Timeout: 10 * time.Second, Transport: t}
	if *record != "" {
		f, err := os.Create(*record)
		if err != nil {
			return err
		}
		defer f.Close()
		w := bufio.NewWriter(f)
		defer w.Flush()
		client.Transport = &recorder{next: t, w: w}
	}
	start := time.Now()
	rep, err := scenario(context.Background(), client, *base, *clients, *n, *seed)
	if err != nil {
		return err
	}
	elapsed := time.Since(start)
	fmt.Printf("clients=%d requests=%d get=%d list=%d create=%d update=%d delete=%d errors=%d rps=%.0f\n",
		*clients, *n, rep.ops[opGet], rep.ops[opList], rep.ops[opCreate], rep.ops[opUpdate], rep.ops[opDelete],
		len(rep.errs), float64(*n)/elapsed.Seconds())
	if len(rep.errs) > 0 {
		return fmt.Errorf("%d unexpected answers, first: %w", len(rep.errs), rep.errs[0])
	}
	return nil
}

type op int

const (
	opGet op = iota
	opList
	opCreate
	opUpdate
	opDelete
)

type report struct {
	ops  [5]int
	errs []error
}

// scenario seeds the items, then runs the mix; a failed seed is the error,
// unexpected answers of the mix are in report.errs.
func scenario(ctx context.Context, client *http.Client, base string, clients, n, seed int) (report, error) {
	if code, body, err := call(ctx, client, "GET", base+"/items?limit=1", ""); err != nil || code != 200 || body != "[]" {
		return report{}, fmt.Errorf("GET /items?limit=1 = %d %q %v, want 200 [] on an empty database", code, body, err)
	}
	for id := 1; id <= seed; id++ {
		code, body, err := call(ctx, client, "POST", base+"/items", itemBody(id))
		if want := seeded(id); err != nil || code != 201 || body != want {
			return report{}, fmt.Errorf("seed POST /items = %d %q %v, want 201 %q", code, body, err, want)
		}
	}

	var mu sync.Mutex
	var rep report
	var wg sync.WaitGroup
	for w := range clients {
		count := n / clients
		if w < n%clients {
			count++
		}
		wg.Go(func() {
			ops, errs := runClient(ctx, client, base, w, count, seed)
			mu.Lock()
			defer mu.Unlock()
			for i, k := range ops {
				rep.ops[i] += k
			}
			rep.errs = append(rep.errs, errs...)
		})
	}
	wg.Wait()
	return rep, nil
}

// runClient sends count requests of the mix as client w.
func runClient(ctx context.Context, client *http.Client, base string, w, count, seed int) ([5]int, []error) {
	rng := rand.New(rand.NewPCG(uint64(w), 1)) //nolint:gosec // G404: a repeatable request mix, not a secret; G115: w is a non-negative client index
	var ops [5]int
	var errs []error
	var own []int // ids this client created and has not deleted
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	for k := range count {
		o := opGet
		switch roll := rng.IntN(100); {
		case roll >= 95:
			o = opDelete
		case roll >= 90:
			o = opUpdate
		case roll >= 80:
			o = opCreate
		case roll >= 60:
			o = opList
		}
		if o == opDelete && len(own) == 0 {
			o = opCreate
		}
		ops[o]++

		switch o {
		case opGet:
			id := 1 + rng.IntN(seed)
			if code, body, err := call(ctx, client, "GET", fmt.Sprintf("%s/items/%d", base, id), ""); err != nil || code != 200 || body != seeded(id) {
				fail("GET /items/%d = %d %q %v, want 200 %q", id, code, body, err, seeded(id))
			}
		case opList:
			after := rng.IntN(seed)
			prefix := fmt.Sprintf(`[{"id":%d,`, after+1)
			code, body, err := call(ctx, client, "GET", fmt.Sprintf("%s/items?after=%d&limit=20", base, after), "")
			if err != nil || code != 200 || !strings.HasPrefix(body, prefix) {
				fail("GET /items?after=%d&limit=20 = %d %q %v, want 200 starting %q", after, code, body, err, prefix)
			}
		case opCreate:
			name := fmt.Sprintf("c%d-%d", w, k)
			code, body, err := call(ctx, client, "POST", base+"/items", fmt.Sprintf(`{"name":%q,"qty":%d}`, name, k))
			var it struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				Qty  int    `json:"qty"`
			}
			if err != nil || code != 201 || json.Unmarshal([]byte(body), &it) != nil || it.ID <= seed || it.Name != name || it.Qty != k {
				fail("POST /items %s = %d %q %v, want 201 with a new id", name, code, body, err)
				continue
			}
			own = append(own, it.ID)
		case opUpdate:
			id := 1 + rng.IntN(seed)
			if code, body, err := call(ctx, client, "PUT", fmt.Sprintf("%s/items/%d", base, id), itemBody(id)); err != nil || code != 200 || body != seeded(id) {
				fail("PUT /items/%d = %d %q %v, want 200 %q", id, code, body, err, seeded(id))
			}
		case opDelete:
			id := own[len(own)-1]
			own = own[:len(own)-1]
			if code, body, err := call(ctx, client, "DELETE", fmt.Sprintf("%s/items/%d", base, id), ""); err != nil || code != 204 {
				fail("DELETE /items/%d = %d %q %v, want 204", id, code, body, err)
			}
		}
	}
	return ops, errs
}

// itemBody is the request body that creates or restores seeded item id.
func itemBody(id int) string { return fmt.Sprintf(`{"name":"item-%d","qty":%d}`, id, id) }

// seeded is the answer for seeded item id.
func seeded(id int) string { return fmt.Sprintf(`{"id":%d,"name":"item-%d","qty":%d}`, id, id, id) }

// call sends one request and returns the status and the trimmed body.
func call(ctx context.Context, client *http.Client, method, url, body string) (int, string, error) {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return 0, "", err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(bytes.TrimSpace(b)), err
}

// recorder writes each request and its response to w: the request line with
// its body, the status, the headers sorted, without Date, and the body.
type recorder struct {
	next http.RoundTripper
	mu   sync.Mutex
	w    io.Writer
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var reqBody []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		reqBody = b
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	resp, err := r.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", req.Method, req.URL.RequestURI())
	if len(reqBody) > 0 {
		fmt.Fprintf(&b, " %s", reqBody)
	}
	fmt.Fprintf(&b, "\n%d\n", resp.StatusCode)
	for _, k := range slices.Sorted(maps.Keys(resp.Header)) {
		if k != "Date" {
			fmt.Fprintf(&b, "%s: %s\n", k, strings.Join(resp.Header[k], ", "))
		}
	}
	fmt.Fprintf(&b, "%s\n\n", body)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := io.WriteString(r.w, b.String()); err != nil {
		return nil, err
	}
	return resp, nil
}
