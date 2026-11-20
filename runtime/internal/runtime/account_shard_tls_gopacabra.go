//go:build gopacabra && !baremetal

package runtime

// currentAccountShard — номер шарду потоку плюс один; 0 — шард ще не
// призначено. Номер, а не вказівник: змінна TLS із вказівником іде в блок
// пакета через LocalPackage, і NoteAlloc на одному ядрі повільніший на 9%.
//
//llgointernal:tls
var currentAccountShard uint32
