//go:build gopacabra

// Програма прийняття P12.5.4: json.Marshal значення форми сервісу rest
// (gopacabra/stand/rest, item). Рядкове поле кодує makeStringArshaler
// (encoding/json/v2/arshal_default.go:231): []byte(s) іде в
// jsonwire.AppendQuote, результат якого лягає в буфер кодувальника. До
// виправлення census вважав, що результат append тримає байти src, і копія
// рядка лишалась живою на кожен виклик. Звільнення місця звіряє
// TestGopacabraJSONQuote за рядками gopacabra site.
package main

import (
	"encoding/json"
	"os"
)

const n = 1000

type item struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Qty  int64  `json:"qty"`
}

func main() {
	const want = `{"id":7,"name":"widget","qty":3}`
	for range n {
		b, err := json.Marshal(item{ID: 7, Name: "widget", Qty: 3})
		if err != nil || string(b) != want {
			println("json.Marshal", string(b), "want", want)
			os.Exit(2)
		}
	}
}
