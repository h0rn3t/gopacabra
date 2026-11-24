//go:build gopacabra

// Програма прийняття P12.5.6.1: буфер подій netpoll (`var events
// [128]epollEvent`) не виділяється в купі на кожен виклик. Його адреса йде
// в C.epoll_wait, тож масив тікає. Рантайм census не розставляє, і місце
// лишається unplaced: ~1.5 КБ без звільнення на кожен виклик netpoll. У gc
// масив живе на стеку. Ехо через loopback паркує горутину на кожному
// читанні, і планувальник M:N кличе netpoll. Тест збирає програму з
// gopacabrastat і читає рядки gopacabra site файлів netpoll.
package main

import (
	"io"
	"net"
	"os"
)

const rounds = 1000

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		println("listen", err.Error())
		os.Exit(1)
	}
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		io.Copy(c, c)
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		println("dial", err.Error())
		os.Exit(1)
	}
	var msg, buf [16]byte
	roundtrip := func() {
		if _, err := c.Write(msg[:]); err != nil {
			println("write", err.Error())
			os.Exit(2)
		}
		if _, err := io.ReadFull(c, buf[:]); err != nil {
			println("read", err.Error())
			os.Exit(2)
		}
	}
	for range rounds {
		roundtrip()
	}
}
