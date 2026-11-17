# OwnGo — звіт фази 11

## P11.3 `GOROOT/test` з тегом `owngo` проти базового llgo

Режим `ci` (`.`, `fixedbugs`, 830 випадків), Go 1.27.1, `linux/amd64`, LLVM 23. Раннер `test/goroot` з `-min-swap-free-mib 0` (на машині розробки вільного свопу ~30 МіБ); ліміт RSS раннера — типовий `-max-rss-mib 4096`. ASan — `-llgo-flags "-tags owngo -O0 -asan"`, `ASAN_OPTIONS=detect_leaks=0`.

### Повні прогони

| SHA | Варіант | `pass` | Регресій проти без тегу |
| --- | --- | --- | --- |
| `192718a3e` | без тегу | 806 | — |
| `192718a3e` | `owngo` | 746 | 60 |
| `192718a3e` | `owngo -O0 -asan` | 735 | 71 (61 `heap-use-after-free`) |
| `f7dddb9bd` | без тегу | 806 | — |
| `f7dddb9bd` | `owngo` | 796 | 10 |
| `f7dddb9bd` | `owngo -O0 -asan` | 796 | 10, жодного `heap-use-after-free`, `double-free` чи `attempting free` |

Без тегу на обох SHA падає `fixedbugs/issue14636.go` і є 1 `XFAIL` — це не регресії `owngo`.

### Підсумковий перезапуск

На `be0894d2f` перезапущено 15 випадків: усі 10 регресій `f7dddb9bd` кожного варіанта. П'ять із них були регресіями лише без ASan, тож обидва списки разом дають 15 випадків. Після `be0894d2f` код ще змінився: `7327b1bb6` прибирає перевірку на `nil` для запису нульового розміру в щойно виділену пам'ять. Поведінки програм ця зміна не змінює.

| Випадок | Без тегу | `owngo` | `owngo -O0 -asan` | Виправлено |
| --- | --- | --- | --- | --- |
| `fixedbugs/bug102.go`, `issue13162.go`, `issue23536.go`, `issue23814.go`, `named.go` | `pass` | `pass` | `pass` | `d339c8237` — `FreeRoot(nil)` |
| `fixedbugs/issue41239.go`, `issue7550.go` | `pass` | `pass` | `pass` | `b4ce48a34` — ємність owned `append` |
| `recover.go` | `pass` | `pass` | `pass` | `b21c0f3eb` — поле нульового розміру у вузлі `defer` |
| `init1.go`, `mallocfin.go`, `tinyfin.go` | `pass` | `unexpected-fail` | `unexpected-fail` | не застосовно, див. нижче |
| `fixedbugs/issue78081.go` | `pass` | `pass` | `resource-fail` | обмеження, див. нижче |
| `inline_callers.go`, `peano.go` | `pass` | `pass` | `unexpected-fail` | обмеження ASan / `-O0` |
| `rangegen.go` | `pass` | `pass` | `resource-fail` | обмеження ASan / `-O0` |

Регресій без причини немає: з `owngo` — 3 випадки, під ASan — 7, і для кожного причина в списку нижче.

### Список з причинами

- **Не застосовно (рішення розробника, 2026-11-03).** `init1.go` вимагає, щоб `runtime.NumGC` зростав. `mallocfin.go` і `tinyfin.go` чекають виклику фіналізаторів `runtime.SetFinalizer`. Під `owngo` трасувального збирача немає за задумом, тож лічильника GC і фіналізаторів теж немає.
- **`fixedbugs/issue78081.go` під ASan — ліміт пам'яті раннера (рішення розробника, 2026-11-03).** 16 горутин по 100 000 разів виконують `*sink = make([]byte, 1024)`. Прохід не звільняє попередній буфер при записі вказівника в пам'ять: таке правило фази 5 у `PLAN.md` («Фаза це правило не послаблює»). Без ASan `owngo live rc-local` на виході показує 1 638 402 272 байти, і випадок проходить. Під ASan із його накладними витратами пам'ять перевищує 4 ГіБ. Звільнення старого значення при перезаписі — можливий наступний крок моделі, не дефект P11.3.
- **Обмеження ASan / `-O0`.** `inline_callers.go`, `peano.go`, `rangegen.go` падають під `-O0 -asan` і без тегу `owngo` (перевірено 2026-11-03), тож це не модель пам'яті. `inline_callers.go` — інші кадри `runtime.Callers` (`main.h main.h main.g` замість `main.h main.g main.f`); `peano.go` — глибока рекурсія, процес завершується з кодом -1; `rangegen.go` — збірка згенерованої програми перевищує ліміт RSS 4 ГіБ.

## P11.5 Перша збірка сервісу — розвідка (2026-11-16)

Сервіс `owngo/stand/rest` на `6d15e50cd`, llgo з HEAD `6fc74217d`, `CGO_ENABLED=0`, LLVM 23, `linux/amd64`. Задачу не закрито: обидві збірки без змін llgo не проходять, а виправлення — кілька окремих задач (нижче), частина з них потребує рішення розробника.

### Що ламається

Усе, крім пункту 5, відтворюється без тегу `owngo`, тобто це помилки базового llgo на `modernc.org/sqlite` v1.62.0 / `modernc.org/libc` v1.80.1.

| # | Де | Симптом | Причина |
| --- | --- | --- | --- |
| 1 | `plan9asm`, `libc/abi0_linux_amd64.s` | збірка: `Y__builtin_mul_overflowUint128: unsupported struct type struct{Lo uint64; Hi uint64}` | файл — 1 875 ABI0-обгорток (`Y*` → `X*`, `__ccgo_abi0_*` → `CALL *AX`), згенерованих `qbecc --abi0wrap`. Транслятор не знає параметрів і результатів-структур (`Uint128`, `div_t`, `ldiv_t`, `imaxdiv_t`, `in_addr`, `ENTRY`, `cookie_io_functions_t`, `sigval`), `complex64`/`complex128` (127 обгорток) і передачі `double` у `CALL` ABI0 (`Y__builtin_copysign: amd64 call unsupported arg type "double"`). 80 функцій з першими двома причинами знайдено за 12 ітерацій з фільтром; скільки ще впаде на `double` — не з'ясовано |
| 2 | `plan9asm`, `libc/tls_linux_amd64.s` (і `abi0`) | збірка: `TLSAlloc: "GO_ARGS": amd64: unsupported instruction GO_ARGS` | `#include "funcdata.h"` пропускається, макроси `GO_ARGS` і `NO_LOCAL_POINTERS` не визначені |
| 3 | `cl`, значення функції | бінарник: nil dereference у `_sqlite3Initialize` → `_sqlite3MutexInit` | ccgo кличе через `(*(*func(...))(unsafe.Pointer(&struct{ uintptr }{fp})))(...)` (~1 250 місць у sqlite, і в libc): у `gc` значення функції — одне слово, вказівник на funcval; у llgo — два слова `{fn, ctx}`, тож читання бере `fn = fp` (адресу даних) і сміття замість `ctx` |
| 4 | `cl`/`ssa`, функція в інтерфейсі | бінарник: SIGSEGV після ~200–360 вставок | `__ccgo_fp(f interface{})` бере слово даних інтерфейсу. llgo кладе статичну функцію в купу (`{fn, nil}`), вказівник живе лише як `uintptr` у пам'яті libc, BDWGC її не бачить і звільняє коробку. У `gc` funcval статичної функції — статичні дані |
| 5 | `owngo`, census | бінарник `owngo`: nil dereference на першому запиті до бази | та сама коробка з п. 4 під `owngo` звільняється після повернення `__ccgo_fp` (`rc-local`) |

Мінімальне відтворення п. 3 (без тегу; `go run` — `5`, `llgo run` — `nil pointer dereference`):

```go
func fp(f interface{}) uintptr { return (*[2]uintptr)(unsafe.Pointer(&f))[1] }
func add(a, b int32) int32     { return a + b }

func main() {
	p := fp(add)
	println((*(*func(int32, int32) int32)(unsafe.Pointer(&struct{ uintptr }{p})))(2, 3))
}
```

### Прототип (не закомічено)

Щоб знати, чи щось лишається за цими п'ятьма пунктами, зібрано тимчасовий llgo: `.s` обох файлів libc пропущено (п. 1–2; на компоновці посилань на `Y*` і `TLSAlloc` не виявилось), а для п. 3–4 — два правила:

- п. 3: читання значення функції (`UnOp *`) через `Convert(unsafe.Pointer → *func)` з `Convert(*T → unsafe.Pointer)`, де `sizeof(T)` — одне слово, читає слово `w` і потім `{fn, ctx}` за `w`. Для правильної llgo-програми такого читання не буває (16 байтів з 8-байтового об'єкта), тож правило нейтральне для upstream;
- п. 4: `MakeInterface` статичної функції — `MakeInterfaceConst`, слово даних — сталий глобал `{fn, nil}` без алокації (лише без `owngo`).

Результат без тегу: програма з 2 000 вставок — `rows 2000`, як у `gc`; сервіс — сценарій `-c 64 -n 100000`: 0 неочікуваних відповідей, 5 959 rps (одиничний прогін, не замір; `gc` — 15 905), зупинка за SIGINT — код 0. З `-tags owngo` збірка — код 0, `nm` — жодного `GC_malloc`/`GC_init`, але перший запит до бази падає (п. 5). Інших помилок за цими п'ятьма прототип не показав; `go test ./cl/` на прототипі не запускався.

### Попередження P10.2

Збірка `-tags owngo` модуля сервісу попереджень не друкує (0 рядків). Місця модуля сервісу: `owngo live` на виході після 1 запиту — `static` 670, `rc-local` 170 115, `rc-shared` 148 834, `cyclic` 15 632, `pinned` 77 455 байтів (бінарник падав, цифри — до розбору P11.6).

## P11.5.0 `owngo` — єдина збірка (2026-11-16)

Рішення розробника: збірка з трасувальним збирачем не цікавить, BDWGC і `tinygogc` видаляються (`INTENT.md`, «`owngo` — єдина збірка»). Зроблено:

- `applyOwngoTag` додає `owngo` до кожної збірки `build.Do`, у `resolveBuildConfig` до вибору ISA (`fd3359da6`; тести — `cea576a23`, `aa79d543c`). Вимкнути нічим.
- Звіт `owngo freed/live` на виході і попередження P10.2 `owngo: pinned` — лише з тегом `owngoreport`; `owngostat` його вмикає (`d904af54c`; тести — `405ab3002`). Без цього кожна програма дописувала звіт у stderr: падали всі тести llgo, що звіряють stderr з `gc` (`cmptest`, `cmd/llgo`, `cmd/internal/run`, `test/cgo`).

### `go test ./...` під типовим `owngo`

Повний прогін (на `cea576a23`, ще зі звітом у stderr), потім перезапуск пакетів, що падали, на `d904af54c` з чистим кешем llgo. Решта 297 пакетів — `ok`.

| Пакет | Після `owngoreport` |
| --- | --- |
| `cmd/llgo`, `cmd/internal/run`, `test/cgo` | `ok` |
| `cmd/internal/build` | `TestRunCmdPassesGoBuildFlags` — середовище (`GOFLAGS=-mod=mod`, відоме) |
| `owngo/mn/callback` | `go test` (не llgo): `C source files not allowed when not using cgo` — середовище, до зміни |
| `cl` | 74 підтести `TestRunAndTest*` і `localitycodegen` |
| `internal/build` | 20 тестів, розклад нижче |

**`cl`: еталони IR під `owngo`.** FileCheck і еталони `cltest` (`_testgo` 31, `_testrt` 23, `_testmeta` 11, `_testdata` 6, `_testlibc` 3, `localitycodegen`) записані для BDWGC-збірки. Під `owngo` інший IR: алокація на стеку замість `AllocZ` (`chan`), опитування `CooperativeSafepoint`/`PreemptRequested` у графі викликів (`_testmeta`), інша нумерація значень і блоків (`concat`, `argv`, `vargs`). Поведінки програм ці перевірки не стосуються; оновлення — за правилом `CLAUDE.md` (найменші рукописні перевірки властивості тесту, `litgen` — для знімків).

**`internal/build`: поведінка рантайму `owngo`, якої тести раніше не бачили.**

| Тест | Симптом | Кошик |
| --- | --- | --- |
| `TestEmbedDirectoryCacheInvalidation` | `panic: open open    xt: file does not exist` — зіпсований рядок імені файлу | дефект `owngo` (схоже на передчасне звільнення) |
| `TestRuntimeCompilerToolchainPackages` | `fatal error: sync: unlock of unlocked mutex` | дефект `owngo` |
| `TestCoverageBuildAgainstGo`, `TestCoverageBuildUncoveredMain`, `TestWorkspaceCoverageAgainstGo` | лічильники покриття не записані на виході (нормальний `return` і `os.Exit`) | дефект `owngo` (шлях виходу) |
| `TestShallowPanicTracebackBuildModes`, `TestTracebackConfiguration` | паніка `main` — `goroutine 2`, не `goroutine 1`; немає кадру паркованої горутини | M:N |
| `TestStrippedSharedLibraryExports` | `c-shared`: `undefined symbol: __llgo_wasm_main` | дефект `owngo` (`c-shared`) |
| `TestPCLNEmbeddedPruningIntegration`, `TestPCLNExternalPruningIntegration`, `TestPCLNExternalPureCLibraryIdentityRetentionIntegration`, `TestMultiBuildRuntimeIsolationAndCache` | програма на чистому C тягне рантайм llgo | `owngo` додає рантайм без потреби |
| `TestSIMD128LLVM` | у IR лишилась гілка перевірки меж для сталої смуги | IR під `owngo`/`v3` |
| `TestWasmGCRootFrameLinksRuntimeChain` | `WASI threads cannot combine nogc with llgo.wasm.gc.linear` | `tinygogc`, P11.5.0.2 |
| `TestBuildDo_DepFingerprintAndVersion`, `TestSIMDFMVLLVM`, `TestSIMDFMVAggregatesLLVM`, `TestSIMDMidwayFMVLLVM` | — | середовище, відомі до зміни |
| `TestRunAndTestFromTestgo` та інші `TestRunAndTest*` у `internal/build` | ті самі еталони IR, що в `cl` | еталони |

Не перевірено: `owngo/fixtures.sh`, `GOROOT/test`, darwin.

## P11.5.0.3 Дефекти рантайму `owngo`: `embed` (2026-11-17)

**`TestEmbedDirectoryCacheInvalidation` — `panic: open <сміття>xt: file does not exist`.** Під `-O0 -asan` — `heap-use-after-free`: `embed.FS.ReadFile` читає ім'я, яке `path.Join` (виклик з `fs.walkDir`) звільнив одразу після `path.Clean(string(buf))`.

Причина — зведення census. `path.Clean` кладе параметр у поле локальної `lazybuf` (`out := lazybuf{s: path}`, SSA: `*&t1.s = path`, `*t0 = *t1`) і повертає `out.string()` — `b.s[:b.w]`. Зведення методу `(*lazybuf).string` — `Alias [0]` (результат — прочитане з `b`), але в `applyCall` результат виклику отримував лише `tok` і `derived` аргументу `t0`, а не його `content`, де й лежить `path`. Тому `Clean` мав порожній `Alias`, і `Join` звільняв `string(buf)` як тимчасовий.

Виправлення (`5265a4b37`): якщо параметр цілі аліасить результат, у результат іде і вміст аргументу (`content[arg]`). Зміна лише збільшує множину аліасів, тож може лише прибрати звільнення.

Тести (`54f7289ec`): `TestLazybufParamAliasesResult` (`owngo/census`), фікстура `owngo/asan/pathjoin` (до виправлення: `Join tree/neste<сміття>`, під ASan — `heap-use-after-free`). Після: обидва зелені, `TestEmbed*` у `internal/build` — `ok`.

Лишились причини P11.5.0.3: `sync: unlock of unlocked mutex` (`TestRuntimeCompilerToolchainPackages`), покриття на виході, `c-shared`, рантайм у програмі на чистому C, траса паніки під M:N.
