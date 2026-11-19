//go:build gopacabra && !gopacabrareport

package runtime

// gopacabraReport вимкнено: gopacabra — єдина збірка, і програма без тегу
// gopacabrareport нічого не дописує в stderr на виході.
const gopacabraReport = false
