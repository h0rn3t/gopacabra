//go:build gopacabra && baremetal

package runtime

// currentAccountShard на baremetal — звичайна глобальна змінна: компонувальник
// xtensa не знає TLS-релокацій, а на ARM без ОС немає __aeabi_read_tp. Спільний
// шард лише зводить лічильники в один слот; самі додавання атомарні.
var currentAccountShard uint32
