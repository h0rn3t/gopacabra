//go:build !gopacabra || !gopacabramn || !(linux || darwin)

package runtime

const gopacabraMNLLGoFiles = ""

// localContextMigrates: горутина завершується на тому самому потоці, де
// почалась, і вкладений вихід перевіряє свій контекст.
const localContextMigrates = false
