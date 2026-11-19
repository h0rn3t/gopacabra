//go:build gopacabra && !gopacabrastat

package runtime

// Без тегу gopacabrastat лічильників операцій немає: порожні тіла компілятор
// вбудовує і прибирає.
func countOp(op int32) {}

func reportOps() {}
