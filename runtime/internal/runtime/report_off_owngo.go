//go:build owngo && !owngoreport

package runtime

// ownGoReport вимкнено: owngo — єдина збірка, і програма без тегу
// owngoreport нічого не дописує в stderr на виході.
const ownGoReport = false
