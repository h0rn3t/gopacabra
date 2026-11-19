//go:build gopacabra && !gopacabrastat

package runtime

// Без тегу gopacabrastat облік задіяних шардів не ведеться: порожнє тіло
// компілятор вбудовує і прибирає.
func noteShard(uint32) {}
