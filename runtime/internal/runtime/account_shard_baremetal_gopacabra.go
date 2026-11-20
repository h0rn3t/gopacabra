//go:build gopacabra && baremetal

package runtime

// currentAccountShard на baremetal — звичайна глобальна змінна: на ARM без ОС
// немає __aeabi_read_tp для TLS. Спільний шард лише зводить лічильники в один
// слот; самі додавання атомарні.
var currentAccountShard uint32
