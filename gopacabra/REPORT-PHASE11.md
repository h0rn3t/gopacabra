# Gopacabra — звіт фази 11

## P11.3 `GOROOT/test` з тегом `gopacabra` проти базового llgo

Режим `ci` (`.`, `fixedbugs`, 830 випадків), Go 1.27.1, `linux/amd64`, LLVM 23. Раннер `test/goroot` з `-min-swap-free-mib 0` (на машині розробки вільного свопу ~30 МіБ); ліміт RSS раннера — типовий `-max-rss-mib 4096`. ASan — `-llgo-flags "-tags gopacabra -O0 -asan"`, `ASAN_OPTIONS=detect_leaks=0`.

### Повні прогони

| SHA | Варіант | `pass` | Регресій проти без тегу |
| --- | --- | --- | --- |
| `192718a3e` | без тегу | 806 | — |
| `192718a3e` | `gopacabra` | 746 | 60 |
| `192718a3e` | `gopacabra -O0 -asan` | 735 | 71 (61 `heap-use-after-free`) |
| `f7dddb9bd` | без тегу | 806 | — |
| `f7dddb9bd` | `gopacabra` | 796 | 10 |
| `f7dddb9bd` | `gopacabra -O0 -asan` | 796 | 10, жодного `heap-use-after-free`, `double-free` чи `attempting free` |

Без тегу на обох SHA падає `fixedbugs/issue14636.go` і є 1 `XFAIL` — це не регресії `gopacabra`.

### Підсумковий перезапуск

На `be0894d2f` перезапущено 15 випадків: усі 10 регресій `f7dddb9bd` кожного варіанта. П'ять із них були регресіями лише без ASan, тож обидва списки разом дають 15 випадків. Після `be0894d2f` код ще змінився: `7327b1bb6` прибирає перевірку на `nil` для запису нульового розміру в щойно виділену пам'ять. Поведінки програм ця зміна не змінює.

| Випадок | Без тегу | `gopacabra` | `gopacabra -O0 -asan` | Виправлено |
| --- | --- | --- | --- | --- |
| `fixedbugs/bug102.go`, `issue13162.go`, `issue23536.go`, `issue23814.go`, `named.go` | `pass` | `pass` | `pass` | `d339c8237` — `FreeRoot(nil)` |
| `fixedbugs/issue41239.go`, `issue7550.go` | `pass` | `pass` | `pass` | `b4ce48a34` — ємність owned `append` |
| `recover.go` | `pass` | `pass` | `pass` | `b21c0f3eb` — поле нульового розміру у вузлі `defer` |
| `init1.go`, `mallocfin.go`, `tinyfin.go` | `pass` | `unexpected-fail` | `unexpected-fail` | не застосовно, див. нижче |
| `fixedbugs/issue78081.go` | `pass` | `pass` | `resource-fail` | обмеження, див. нижче |
| `inline_callers.go`, `peano.go` | `pass` | `pass` | `unexpected-fail` | обмеження ASan / `-O0` |
| `rangegen.go` | `pass` | `pass` | `resource-fail` | обмеження ASan / `-O0` |

Регресій без причини немає: з `gopacabra` — 3 випадки, під ASan — 7, і для кожного причина в списку нижче.

### Список з причинами

- **Не застосовно (рішення розробника, 2026-11-03).** `init1.go` вимагає, щоб `runtime.NumGC` зростав. `mallocfin.go` і `tinyfin.go` чекають виклику фіналізаторів `runtime.SetFinalizer`. Під `gopacabra` трасувального збирача немає за задумом, тож лічильника GC і фіналізаторів теж немає.
- **`fixedbugs/issue78081.go` під ASan — ліміт пам'яті раннера (рішення розробника, 2026-11-03).** 16 горутин по 100 000 разів виконують `*sink = make([]byte, 1024)`. Прохід не звільняє попередній буфер при записі вказівника в пам'ять: таке правило фази 5 у `PLAN.md` («Фаза це правило не послаблює»). Без ASan `gopacabra live rc-local` на виході показує 1 638 402 272 байти, і випадок проходить. Під ASan із його накладними витратами пам'ять перевищує 4 ГіБ. Звільнення старого значення при перезаписі — можливий наступний крок моделі, не дефект P11.3.
- **Обмеження ASan / `-O0`.** `inline_callers.go`, `peano.go`, `rangegen.go` падають під `-O0 -asan` і без тегу `gopacabra` (перевірено 2026-11-03), тож це не модель пам'яті. `inline_callers.go` — інші кадри `runtime.Callers` (`main.h main.h main.g` замість `main.h main.g main.f`); `peano.go` — глибока рекурсія, процес завершується з кодом -1; `rangegen.go` — збірка згенерованої програми перевищує ліміт RSS 4 ГіБ.

## P11.5 Перша збірка сервісу — розвідка (2026-11-16)

Сервіс `gopacabra/stand/rest` на `6d15e50cd`, llgo з HEAD `6fc74217d`, `CGO_ENABLED=0`, LLVM 23, `linux/amd64`. Задачу не закрито: обидві збірки без змін llgo не проходять, а виправлення — кілька окремих задач (нижче), частина з них потребує рішення розробника.

### Що ламається

Усе, крім пункту 5, відтворюється без тегу `gopacabra`, тобто це помилки базового llgo на `modernc.org/sqlite` v1.62.0 / `modernc.org/libc` v1.80.1.

| # | Де | Симптом | Причина |
| --- | --- | --- | --- |
| 1 | `plan9asm`, `libc/abi0_linux_amd64.s` | збірка: `Y__builtin_mul_overflowUint128: unsupported struct type struct{Lo uint64; Hi uint64}` | файл — 1 875 ABI0-обгорток (`Y*` → `X*`, `__ccgo_abi0_*` → `CALL *AX`), згенерованих `qbecc --abi0wrap`. Транслятор не знає параметрів і результатів-структур (`Uint128`, `div_t`, `ldiv_t`, `imaxdiv_t`, `in_addr`, `ENTRY`, `cookie_io_functions_t`, `sigval`), `complex64`/`complex128` (127 обгорток) і передачі `double` у `CALL` ABI0 (`Y__builtin_copysign: amd64 call unsupported arg type "double"`). 80 функцій з першими двома причинами знайдено за 12 ітерацій з фільтром; скільки ще впаде на `double` — не з'ясовано |
| 2 | `plan9asm`, `libc/tls_linux_amd64.s` (і `abi0`) | збірка: `TLSAlloc: "GO_ARGS": amd64: unsupported instruction GO_ARGS` | `#include "funcdata.h"` пропускається, макроси `GO_ARGS` і `NO_LOCAL_POINTERS` не визначені |
| 3 | `cl`, значення функції | бінарник: nil dereference у `_sqlite3Initialize` → `_sqlite3MutexInit` | ccgo кличе через `(*(*func(...))(unsafe.Pointer(&struct{ uintptr }{fp})))(...)` (~1 250 місць у sqlite, і в libc): у `gc` значення функції — одне слово, вказівник на funcval; у llgo — два слова `{fn, ctx}`, тож читання бере `fn = fp` (адресу даних) і сміття замість `ctx` |
| 4 | `cl`/`ssa`, функція в інтерфейсі | бінарник: SIGSEGV після ~200–360 вставок | `__ccgo_fp(f interface{})` бере слово даних інтерфейсу. llgo кладе статичну функцію в купу (`{fn, nil}`), вказівник живе лише як `uintptr` у пам'яті libc, BDWGC її не бачить і звільняє коробку. У `gc` funcval статичної функції — статичні дані |
| 5 | `gopacabra`, census | бінарник `gopacabra`: nil dereference на першому запиті до бази | та сама коробка з п. 4 під `gopacabra` звільняється після повернення `__ccgo_fp` (`rc-local`) |

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
- п. 4: `MakeInterface` статичної функції — `MakeInterfaceConst`, слово даних — сталий глобал `{fn, nil}` без алокації (лише без `gopacabra`).

Результат без тегу: програма з 2 000 вставок — `rows 2000`, як у `gc`; сервіс — сценарій `-c 64 -n 100000`: 0 неочікуваних відповідей, 5 959 rps (одиничний прогін, не замір; `gc` — 15 905), зупинка за SIGINT — код 0. З `-tags gopacabra` збірка — код 0, `nm` — жодного `GC_malloc`/`GC_init`, але перший запит до бази падає (п. 5). Інших помилок за цими п'ятьма прототип не показав; `go test ./cl/` на прототипі не запускався.

### Попередження P10.2

Збірка `-tags gopacabra` модуля сервісу попереджень не друкує (0 рядків). Місця модуля сервісу: `gopacabra live` на виході після 1 запиту — `static` 670, `rc-local` 170 115, `rc-shared` 148 834, `cyclic` 15 632, `pinned` 77 455 байтів (бінарник падав, цифри — до розбору P11.6).

## P11.5.0 `gopacabra` — єдина збірка (2026-11-16)

Рішення розробника: збірка з трасувальним збирачем не цікавить, BDWGC і `tinygogc` видаляються (`INTENT.md`, «`gopacabra` — єдина збірка»). Зроблено:

- `applyGopacabraTag` додає `gopacabra` до кожної збірки `build.Do`, у `resolveBuildConfig` до вибору ISA (`fd3359da6`; тести — `cea576a23`, `aa79d543c`). Вимкнути нічим.
- Звіт `gopacabra freed/live` на виході і попередження P10.2 `gopacabra: pinned` — лише з тегом `gopacabrareport`; `gopacabrastat` його вмикає (`d904af54c`; тести — `405ab3002`). Без цього кожна програма дописувала звіт у stderr: падали всі тести llgo, що звіряють stderr з `gc` (`cmptest`, `cmd/llgo`, `cmd/internal/run`, `test/cgo`).

### `go test ./...` під типовим `gopacabra`

Повний прогін (на `cea576a23`, ще зі звітом у stderr), потім перезапуск пакетів, що падали, на `d904af54c` з чистим кешем llgo. Решта 297 пакетів — `ok`.

| Пакет | Після `gopacabrareport` |
| --- | --- |
| `cmd/llgo`, `cmd/internal/run`, `test/cgo` | `ok` |
| `cmd/internal/build` | `TestRunCmdPassesGoBuildFlags` — середовище (`GOFLAGS=-mod=mod`, відоме) |
| `gopacabra/mn/callback` | `go test` (не llgo): `C source files not allowed when not using cgo` — середовище, до зміни |
| `cl` | 74 підтести `TestRunAndTest*` і `localitycodegen` |
| `internal/build` | 20 тестів, розклад нижче |

**`cl`: еталони IR під `gopacabra`.** FileCheck і еталони `cltest` (`_testgo` 31, `_testrt` 23, `_testmeta` 11, `_testdata` 6, `_testlibc` 3, `localitycodegen`) записані для BDWGC-збірки. Під `gopacabra` інший IR: алокація на стеку замість `AllocZ` (`chan`), опитування `CooperativeSafepoint`/`PreemptRequested` у графі викликів (`_testmeta`), інша нумерація значень і блоків (`concat`, `argv`, `vargs`). Поведінки програм ці перевірки не стосуються; оновлення — за правилом `CLAUDE.md` (найменші рукописні перевірки властивості тесту, `litgen` — для знімків).

**`internal/build`: поведінка рантайму `gopacabra`, якої тести раніше не бачили.**

| Тест | Симптом | Кошик |
| --- | --- | --- |
| `TestEmbedDirectoryCacheInvalidation` | `panic: open open    xt: file does not exist` — зіпсований рядок імені файлу | дефект `gopacabra` (схоже на передчасне звільнення) |
| `TestRuntimeCompilerToolchainPackages` | `fatal error: sync: unlock of unlocked mutex` | дефект `gopacabra` |
| `TestCoverageBuildAgainstGo`, `TestCoverageBuildUncoveredMain`, `TestWorkspaceCoverageAgainstGo` | лічильники покриття не записані на виході (звичайний `return` з `main`; `os.Exit` пише) | дефект `gopacabra` (шлях виходу M:N) |
| `TestShallowPanicTracebackBuildModes`, `TestTracebackConfiguration` | паніка `main` — `goroutine 2`, не `goroutine 1`; немає кадру паркованої горутини | M:N |
| `TestStrippedSharedLibraryExports` | `c-shared`: `undefined symbol: __llgo_wasm_main` | дефект `gopacabra` (`c-shared`) |
| `TestPCLNEmbeddedPruningIntegration`, `TestPCLNExternalPruningIntegration`, `TestPCLNExternalPureCLibraryIdentityRetentionIntegration`, `TestMultiBuildRuntimeIsolationAndCache` | програма на чистому C тягне рантайм llgo | `gopacabra` додає рантайм без потреби |
| `TestSIMD128LLVM` | у IR лишилась гілка перевірки меж для сталої смуги | IR під `gopacabra`/`v3` |
| `TestWasmGCRootFrameLinksRuntimeChain` | `WASI threads cannot combine nogc with llgo.wasm.gc.linear` | `tinygogc`, P11.5.0.2 |
| `TestBuildDo_DepFingerprintAndVersion`, `TestSIMDFMVLLVM`, `TestSIMDFMVAggregatesLLVM`, `TestSIMDMidwayFMVLLVM` | — | середовище, відомі до зміни |
| `TestRunAndTestFromTestgo` та інші `TestRunAndTest*` у `internal/build` | ті самі еталони IR, що в `cl` | еталони |

Не перевірено: `gopacabra/fixtures.sh`, `GOROOT/test`, darwin.

## P11.5.0.3 Дефекти рантайму `gopacabra`: `embed` (2026-11-17)

**`TestEmbedDirectoryCacheInvalidation` — `panic: open <сміття>xt: file does not exist`.** Під `-O0 -asan` — `heap-use-after-free`: `embed.FS.ReadFile` читає ім'я, яке `path.Join` (виклик з `fs.walkDir`) звільнив одразу після `path.Clean(string(buf))`.

Причина — зведення census. `path.Clean` кладе параметр у поле локальної `lazybuf` (`out := lazybuf{s: path}`, SSA: `*&t1.s = path`, `*t0 = *t1`) і повертає `out.string()` — `b.s[:b.w]`. Зведення методу `(*lazybuf).string` — `Alias [0]` (результат — прочитане з `b`), але в `applyCall` результат виклику отримував лише `tok` і `derived` аргументу `t0`, а не його `content`, де й лежить `path`. Тому `Clean` мав порожній `Alias`, і `Join` звільняв `string(buf)` як тимчасовий.

Виправлення (`5265a4b37`): якщо параметр цілі аліасить результат, у результат іде і вміст аргументу (`content[arg]`). Зміна лише збільшує множину аліасів, тож може лише прибрати звільнення.

Тести (`54f7289ec`): `TestLazybufParamAliasesResult` (`gopacabra/census`), фікстура `gopacabra/asan/pathjoin` (до виправлення: `Join tree/neste<сміття>`, під ASan — `heap-use-after-free`). Після: обидва зелені, `TestEmbed*` у `internal/build` — `ok`.

Лишились причини P11.5.0.3: `sync: unlock of unlocked mutex` (`TestRuntimeCompilerToolchainPackages`), покриття на виході, `c-shared`, рантайм у програмі на чистому C, траса паніки під M:N.

## P11.5.0.3 Дефекти рантайму `gopacabra`: `sync: unlock of unlocked mutex` (2026-11-17)

**`TestRuntimeCompilerToolchainPackages` — `fatal error: sync: unlock of unlocked mutex`** (у цій сесії симптом був інший — `nil pointer dereference` в `os.Stat` з `internal/exportdata.FindPkg`). Під `-O0 -asan` — `heap-use-after-free` у `sync.(*Once).Do`: `internal/exportdata.lookupGorootExport` кладе в `sync.Map` замикання, що захопило `listOnce`, `exportPath` і `err`, і викликає його через результат `LoadOrStore`. Комірки захоплених змінних звільнялись на виході `lookupGorootExport`, а замикання лишалось у мапі; наступний `go/importer.Import` читав звільнений `sync.Once`.

Причина — `transfer` у census. `LoadOrStore` зберігає параметр у пам'яті приймача і повертає його (`Side` — `call`, результат аліасить параметр). Сам аргумент (замикання) отримував `Inc` у виклику, але те, що він тримає — комірки замикання, поля структури, — лишалось з `Dec` на виході функції: `Dec` вмісту за звільненням об'єкта не йде. Для `return` це вже враховано (`exitSites` знімає `Dec` з усього досяжного з результату), для аргументу, який callee зберіг, — ні. Те саме — для callee без зведення (виклик через інтерфейс): там `Dec` знімався лише з самого аргументу. Дірка не специфічна для `sync.Map`: відтворюється і на власному `LoadOrStore` з `map[string]any`, і на `&T{s: &s}` замість замикання.

Виправлення (`264fd7edc`): `blockHeld` — якщо callee зберігає аргумент (`Side != 0`) або зведення невідоме, з усього, що досяжне з вмісту аргументу (крім самого аргументу), знімається `Dec`. Обхід — спільний з `exitSites` (`reach`). Зміна лише прибирає `Dec`, тож може дати витік, але не звільнення живого. Callee, що лише читає аргумент (`Side == 0`), `Dec` вмісту не втрачає.

Тести (`680aa3650`): `TestKeptArgumentContentsLive` (`gopacabra/census`; до виправлення `closure`, `field`, `unknown` — `Dec` на комірці), фікстура `gopacabra/asan/keptclosure` (до виправлення без ASan — `nil pointer dereference`, під ASan — `heap-use-after-free`). Після: обидва зелені; `TestRuntimeCompilerToolchainPackages` і `TestEmbed*` (`internal/build`) — `ok`; `go test ./cl/ -run Gopacabra` з чистим кешем — `ok`; `fixtures.sh` — 0 з 88.

Лишились причини P11.5.0.3: покриття на виході, `c-shared`, рантайм у програмі на чистому C, траса паніки під M:N.

## P11.5.0.3 Дефекти рантайму `gopacabra`: покриття на виході (2026-11-17)

**`TestCoverageBuildAgainstGo`, `TestCoverageBuildUncoveredMain`, `TestWorkspaceCoverageAgainstGo` — лічильники покриття не записані.** Уточнення до розкладу P11.5.0: `os.Exit` лічильники пише (через `os.runtime_beforeExit`), не пише лише звичайне повернення з `main`. Під `gopacabra1to1` повернення з `main` теж пише — дефект у моделі M:N.

Причина — точка входу. `runtime.runCoverageExitHook` для звичайного повернення компілятор кликав у хвості точки входу після `main.main` (`emitRuntimeMainBody`). Під `gopacabramn` ініціалізація і `main.main` ідуть у задачі `__llgo_wasm_main` на окремій G, а `mnGEntry` після повернення з `main` викликає `ReportExit` і `c.Exit(0)` прямо з цієї G: хвіст точки входу після `RunWasmMain` не виконується. Звіт `gopacabra` (`ReportExit`) рантайм уже кликав сам, хук покриття — ні.

Виправлення (`419cfe70a`): `defineWasmMainTask` кличе хук у задачі одразу після `main.main`, як `runtime.main` у Go; хвіст точки входу кличе його лише для збірок без задачі. Для wasm з асинхронним планувальником хук теж переходить у задачу — порядок той самий (після `main.main`, до виходу).

Тести (`00c697e52`): `TestGenMainModuleCoverageExitMN` (`internal/build`; до виправлення в задачі немає виклику хука). Після: зелений, як і `TestCoverageBuild*`, `TestWorkspaceCoverageAgainstGo` та решта `TestCoverage*`, `TestGenMainModule*`.

Лишились причини P11.5.0.3: `c-shared`, рантайм у програмі на чистому C, траса паніки під M:N.

## P11.5.0.3 Дефекти рантайму `gopacabra`: `c-shared` (2026-11-17)

**`TestStrippedSharedLibraryExports` — `dlopen`: `undefined symbol: __llgo_wasm_main`.** `applyGopacabraTag` давав `gopacabramn` кожній нативній збірці, і бібліотечним режимам теж. Рантайм M:N (`proc_mn_gopacabra.go`) посилається на задачу `__llgo_wasm_main` через `RunWasmMain`, а компілятор визначає її лише для `exe` (`needsWasmRuntimeScheduler`). Символ лишався невизначеним, і `dlopen(RTLD_NOW)` падав.

Причина глибша за символ: M:N стартує з точки входу `exe` (`RunWasmMain` робить головний потік M0 і запускає планувальник). Бібліотека точкою входу процесу не володіє, її експорти виконуються на потоках хоста, тож планувальник M:N у ній не запускається ніколи. Модель 1:1 (потік на горутину) з потоками хоста вже працює: експорт `c-shared`, що запускає горутину і чекає на каналі, під `gopacabra1to1` повертав правильне значення.

Виправлення (`90139a1ae`): `applyGopacabraTag` додає `gopacabramn` лише для `exe` (порожній `BuildMode` — теж `exe`); `c-shared` і `c-archive` лишаються на 1:1. Заглушка `__llgo_wasm_main` у бібліотеці чи передача задачі параметром лише прибрали б симптом: планувальник у бібліотеці однаково не стартує.

Тести (`70b96af3e`): `TestApplyGopacabraTag` (рядки `exe`, `c-shared`, `c-archive`; до виправлення бібліотеки отримували `gopacabramn`), `TestGopacabraCSharedGoroutine` (програма на C завантажує бібліотеку і кличе експорт з горутиною; до виправлення — `undefined symbol: __llgo_wasm_main`). Після: обидва зелені, як і `TestStrippedSharedLibraryExports`, `TestGenMainModule*`. Та сама бібліотека під `-O0 -asan` (з `LD_PRELOAD` рантайму ASan) — `42`, без звіту.

Лишились причини P11.5.0.3: рантайм у програмі на чистому C (`TestPCLN*Pruning*`, `TestPCLNExternalPureCLibraryIdentityRetentionIntegration`, `TestMultiBuildRuntimeIsolationAndCache`), траса паніки під M:N.

## P11.5.0.3 Дефекти рантайму `gopacabra`: рантайм у програмі на чистому C (2026-11-17)

**`TestPCLNExternalPureCLibraryIdentityRetentionIntegration` — `pure lib/c fixture unexpectedly linked the LLGo runtime`; `TestMultiBuildRuntimeIsolationAndCache` — `cprintf unexpectedly requires runtime`.** Програма `c.Printf(c.Str(...))` під типовою збіркою (`gopacabramn`) мала 192 символи `runtime/internal/runtime.`, під `gopacabra1to1` — один (`ReportExit`).

Причина — кодогенерація M:N, а не рантайм. Під `gopacabramn` компілятор ставить у кожну функцію опитування `CooperativeSafepoint` (через `PreemptRequested`) і обгортає виклики C у `EnterSyscall`/`ExitSyscall`. Тому пакет `main` отримує `NeedRt`, хоча нічого з рантайму не використовує. Точка входу додатково запускає планувальник (`RunWasmMain`, задача `__llgo_wasm_main`) і викликає `ReportExit`. Програмі без рантайму другої горутини не завести: опитування, позначки і планувальник у ній зайві.

Виправлення (`c8868fe8e`): доказ upstream «програма без рантайму» (`configureWasmRuntimeFree`, досі лише для WASI) розділено на ціле-специфічну частину і спільну `provenRuntimeFree`: обхід SSA від `init`/`main` і перевірку пакетів на код C, експорти та `_wrap`. `configureGopacabraMNRuntimeFree` застосовує доказ до нативної `exe` під `gopacabramn`. Якщо він проходить, вимикаються опитування, `PreemptRequested` і позначки викликів C, а точка входу не запускає планувальник і не викликає `ReportExit`. Ознака `GOPACABRA_MN_RUNTIME_FREE` входить у відбиток кешу. Кілька початкових пакетів з різним результатом доказу діляться на групи (`groupGopacabraMNRuntimeFree`, механізм груп upstream), бо доказ змінює код пакетів: програма без рантайму не ділить архіви з програмою, якій він потрібен. Доказ консервативний: за `-O0`, з покриттям чи з `LDFLAGS` він не застосовується, і програма лишається на M:N, як раніше.

Тести (`23510ac67`): `TestGopacabraMNRuntimeFreeProgram` (`internal/build`). Підтест `pure-c` — без символів рантайму (до виправлення — з ними); `goroutine` — контроль, рантайм і M:N на місці. Після виправлення зелені `TestGopacabraMNRuntimeFreeProgram`, `TestPCLNExternalPureCLibraryIdentityRetentionIntegration`, `TestPCLNEmbeddedPruningIntegration`, `TestPCLNExternalPruningIntegration`, `TestMultiBuildRuntimeIsolationAndCache` і тести груп (`TestMixedInitialGroups*`, `TestInitialGroup*`). Тести доказу wasm і відбитка (`RuntimeFree|Fingerprint|Wasm.*Reflect`) зелені, крім відомого `TestBuildDo_DepFingerprintAndVersion` (падає й до зміни). Та сама програма під `-O0 -asan` — вивід правильний, звіту ASan немає.

Лишилась причина P11.5.0.3: траса паніки під M:N (`TestShallowPanicTraceback*`, `TestTracebackConfiguration`).

## P11.5.0.3 Дефекти рантайму `gopacabra`: траса паніки під M:N (2026-11-18)

**`TestShallowPanicTracebackBuildModes` — `panic traceback is missing "goroutine 1 [running]:"`; `TestTracebackConfiguration` — немає `main.parkedWorker(` у `GOTRACEBACK=all`.** Під `gopacabramn` паніка `main` друкувалась як `goroutine 2`. Паркована горутина мала лише `goroutine running on other thread; stack unavailable`. Ще з'являвся зайвий `goroutine 1 [running]` зі сміттєвими кадрами (`abi.splitName +0x7fa8…`).

Причина — реєстр трас (`runtime/internal/stacktrace`) розрахований на модель «потік на горутину». Знімок інших горутин надсилає `SIGURG` потоку кожного вузла і розгортає стек у його обробнику. Під M:N так не працює з двох причин:
- **Номери.** Початкова G головного потоку створюється ліниво з номером 1, а `RunWasmMain` робить її g0 потоку M0. `main` запускався новою G з номером 2. Вузол g0 лишався прив'язаним до головного потоку, і знімок розгортав стек цього потоку, на якому в той момент виконувалась зовсім інша G. Звідси сміттєві кадри.
- **Стек паркованої G.** G на стеку M:N не має свого потоку, її вузол ніколи не прив'язувався, тож знімок не мав звідки взяти кадри.

Виправлення (`da4fb8bbf`):
- `RunWasmMain` віддає номер 1 головній G і створює її без батька (без «created by», як у Go). Початкова G стає g0 з номером 0. Вузли з номером 0 знімок пропускає: це рідний стек потоку планувальника, не горутина.
- Вузол G під M:N отримує збережений контекст і вказівник на `atomicstatus` (`stacktrace.Saved`, `llgo_traceback_saved`). Для G, що не виконується, знімок розгортає стек від адреси повернення і `fp`, які `gopacabra_mn_swap` зберіг у кадрі перемикання. Розгортання йде ланцюгом кадрів у межах стека G. Читач кадру (`mn_saved_frame`) живе в `gopacabra_mn.c`, де відоме компонування кадру `swap` для x86-64 і arm64, і реєструється в `stacktrace` при першому контексті. Для G, що виконується на іншій M, лишається «stack unavailable», як у `gc` без зупинки світу.
- `mnGoexit0` знімає вузол з реєстру до звільнення стека: знімок читає чужий стек під замком реєстру, тож стек не може зникнути посеред обходу.

Тести: червоні до виправлення `TestShallowPanicTracebackBuildModes` і `TestTracebackConfiguration` (їхні фікстури — `internal/build/testdata/shallowpanic` і `tracebackconfig`); окремої фікстури не додано. Після виправлення обидва зелені. Зелені також тести `internal/build` за `Traceback|Panic|Stack|Goroutine|Debugger|AllThreads|Fault|Crash|Signal|Caller|PCLN` і `go test ./cl/ -run Gopacabra` з чистим кешем. `tracebackconfig` під `-O0 -asan` у режимах `all`, `system` і `runtime.Stack(all)` дає `goroutine 1 [running]` і `goroutine 3 [chan receive]` з `main.parkedWorker` без звіту ASan.

Що не змінено, бо так само поводиться модель 1:1: кадр обгортки `go` атрибутований рядку виклику, тож `main.parkedWorker` видно двічі; горутина таймерів видна як `running` з «stack unavailable». `AllThreadsSyscall` під M:N не перевірявся: вузли G без потоку він бачить як неприв'язані (`EAGAIN`), і так було до цієї зміни.

Усі причини P11.5.0.3 закрито.

## P11.5.0.4 Еталони IR під `gopacabra` (2026-11-19)

74 підтести `cl` `TestRunAndTest*` і `localitycodegen`, а також `internal/build` `TestSIMD128LLVM`, перевіряли IR збірки BDWGC. Під `gopacabra` IR інший з трьох причин, жодна не змінює поведінки програм (`expect.txt` усіх фікстур не змінювались):

- **Опитування M:N.** Кожна не-листова функція починається з `load atomic i32 PreemptRequested` → `icmp ne` → `br` у блок `CooperativeSafepoint`, далі блок злиття; цикли з необмеженим зворотним ребром мають таке саме. Ланцюги `CHECK-NEXT` після мітки входу рвались, нумерація значень і блоків зсувалась.
- **Алокація.** Escape-аналіз кладе колишні `AllocZ`/`AllocU` на стек (`alloca` + `llvm.memset`); купа — з `NoteAlloc`, звільнення — `FreeAccounted`/`DecAccounted`, лічильні й регіональні алокатори (`AllocCounted`, `RegionAlloc`), `EnterSyscall`/`ExitSyscall` навколо викликів C.
- **gls під M:N.** Змінні горутини — у `__llgo_gls_block` через `GoroutineLocalPackage` з ключем `@main.__llgo_gls_key`, не `thread_local`.

Що зроблено (правило `CLAUDE.md`, «Maintain LLVM IR checks by test intent»):

| Коміт | Що |
| --- | --- |
| `3020f6ff1` | еталони `meta-expect.txt` 12 фікстур `_testmeta` (разом з `api/`, `model/`): граф викликів містить `CooperativeSafepoint`/`PreemptRequested`. Узяті з `.new`, які пише сам тест; `chore/gentests` не годиться — обрізає `meta` і друкує float інакше |
| `7f497dac4` | 14 знімків `litgen` (`go run ./chore/litgen -u cl`); `embedunexport-1598` лишився як був |
| `a2ce3815f` | `_testdata` (`cpkg`, `embedunexport`, `llgointrinsics`, `vargs`), `_testlibc` (`argv`, `defer`, `setjmp`), `localitycodegen` |
| `89b9ee1fd`, `600bc0570` | 24 фікстури `_testgo` |
| `f06250b66` | 15 фікстур `_testrt` |
| `cd25eeede` | `tpinst`, `typerecur`, `tpabi` (з першого прогону, пропущені при розподілі) |
| `39ea29d04` | `TestSIMD128LLVM` |

Для кожної рукописної фікстури спершу названо властивість, яку вона захищає, і лишено найменші перевірки цієї властивості: `CHECK-NEXT` після входу — `CHECK`; перевірки, що фіксували лише форму `AllocZ`/`AllocU` у тестах не про алокацію, прибрано (причина — у коміті); у тестах про алокацію, escape чи середовище замикання — перевірено форму `gopacabra`. Рукописні тести на `litgen` не переводились.

`localitycodegen` тепер перевіряє `LocalPackage(..., i64 8, i64 8)`, `GoroutineLocalPackage(ptr @main.__llgo_gls_key, i64 8, i64 8)` і `__llgo_gls_block`. Дві старі перевірки `CHECK-NOT: @main.pointer = thread_local` і `@main.initialized = thread_local` не могли спрацювати ні за якого IR; їх виправлено.

**`TestSIMD128LLVM`.** Властивість «стала смуга без гілки перевірки меж» під `gopacabra` тримається: `main.fixed` — `extractelement <4 x float> %0, i32 1` без `icmp uge` і `PanicSIMDImmediate` на amd64 і arm64. Тест падав, бо шукав будь-який `br` — а це гілка опитування. Так само перевірка «перетворення маски без зовнішнього виклику» ловила `call CooperativeSafepoint`. Тепер тест шукає саме гілку меж (`icmp uge`, `PanicSIMDImmediate`) і вимагає сталу смугу в `extractelement`; серед викликів не враховує лише `CooperativeSafepoint`.

У таблиці P11.5.0 рядок «`TestRunAndTestFromTestgo` та інші `TestRunAndTest*` у `internal/build`» помилковий: таких тестів у `internal/build` немає, вони лише в `cl/compile_test.go`.

### Прийняття

- `go test -count=1 -timeout 90m ./cl/` з чистим `~/.cache/llgo` — `ok` (721 с).
- `go run ./chore/litgen -u cl` змінює лише `embedunexport-1598` (відомий виняток; зміну відкинуто). `litgen -u --check cl` зупиняється на першому застарілому файлі, тож решту знімків перевірено через `-u` і `git status`.
- `go test ./internal/build/` (44 хв): падають `TestBuildDo_DepFingerprintAndVersion`, `TestSIMDFMVLLVM`, `TestSIMDFMVAggregatesLLVM`, `TestSIMDMidwayFMVLLVM` (середовище, відомі до P11.5.0) і `TestWasmGCRootFrameLinksRuntimeChain` (`tinygogc`, P11.5.0.2). `TestSIMD128LLVM` — зелений на amd64, arm64, wasm.

Цим закрито всі падіння таблиці P11.5.0, крім `TestWasmGCRootFrameLinksRuntimeChain` (P11.5.0.2) і падінь середовища.

### Помічене в IR, не виправлене

Не дефекти поведінки (тести поведінки зелені); кандидати на окремі задачі, не перевірені:

- Відкладені виклики C (`defer C.printf(...)`) ідуть без `EnterSyscall`/`ExitSyscall`: блокувальний відкладений виклик C тримає M. Так само `_Cfunc_free` (результат `[0 x i8]`) не обгорнуто.
- Вхід `_Cfunc_*` має невикористаний `AllocZ` з `NoteAlloc(..., i32 4)`.
- `alloca` кадру `Defer` стоїть у блоці злиття після опитування, а не в блоці входу: `mem2reg`/статичний кадр не застосовуються, у циклі стек може рости.
- `cgobasic`: блоки `PanicIndex` закінчуються `br` на себе (замість `unreachable`).
- Бокси `AllocU` звільняються `FreeAccounted(ptr, i64 0, i32 0)` — розмір 0 в обліку.
- `typalias`: `&Foo{}` лишається в купі — немає міжпроцедурного escape.
- Листова за суттю `main.fixed` (SIMD) отримує вхідне опитування — зайва ціна на гарячих коротких функціях.

## P11.5.0.1 Видалення BDWGC (2026-11-20)

Кожна збірка має тег `gopacabra`, а з ним `nogc`, тож файли рантайму з `!nogc` на нативних цілях були мертвим кодом. BDWGC під'єднувався лише через `runtime/internal/clite/bdwgc`.

| Коміт | Що |
| --- | --- |
| `7903d9a95` | тест спершу: `TestRuntimeLinksNoTracingGC` — бінарник `hello` не має `libgc` серед `NEEDED` |
| `03445b436` | видалено пакет `bdwgc`, `!nogc`-файли рантайму, тести лише про BDWGC; залежність `bdw-gc` з `install.sh`, `Dockerfile`, `dev/`, `.github`, `.goreleaser.yaml`. Тести фіналізаторів (`test/go/finalizer_test.go` тощо) — `//go:build !nogc` з причиною: без трасувального збирача `SetFinalizer` — no-op |
| `e9c66116e` | нативні цілі обирають nogc-файли рантайму й без тегу `nogc` |
| `f5d232a58` | README, `CLAUDE.md`: BDWGC видалено |

Разом до документації: 71 файл, +131/−2412 рядків, 24 файли видалено.

**Чому потрібен `e9c66116e`.** Без `!nogc`-файлів рантайм, завантажений без тегу `nogc`, неповний. Так його вантажать `importer.For("source")` у тестах `cl` (`TestArrayCompareReusesImmutableLocalStorage` — паніка на nil-об'єкті `AllocU`, що валила весь тестовий бінарник `cl`) і `dev/runtimeaudit` (19 неприв'язаних посилань). Тепер нативні nogc-файли мають умову `(nogc || !baremetal)` або `!baremetal`. `nogc` лишається розрізненням для baremetal і wasm `llgo.wasm.gc.linear` — це `tinygogc`, P11.5.0.2. До меж платформи в `internal/build/runtime_audit_boundaries.go` додано `C.posix_memalign`. У `dev/runtimeaudit` сирий js/wasm тепер звітує «runtime boundaries without a provider» (відомий брак провайдера `C.llgo_js_random_data`), а не «unresolved runtime references».

### Що лишилось у `git grep -i 'bdwgc\|GC_malloc\|GC_init\|libgc'`

Поза `PLAN.md`, `PROGRESS.md`, `INTENT.md` і звітами (історія):

| Файл | Причина |
| --- | --- |
| `README.md`, `CLAUDE.md` | речення «BDWGC видалено» |
| `LLGO.md` | посилання на зовнішній біндинг `goplus/lib` (не код llgo) |
| `internal/build/package_archive.go` | коментар upstream про обхідний шлях; diff від upstream мінімальний |
| `internal/build/gopacabra_test.go` | сам тест-сторож (`libgc`) |
| `gopacabra/bench/allocbase` | бенчмарк порівнює з `libgc` свідомо |
| `runtime/internal/runtime/tinygogc/*` | коментарі, що згадують BDWGC, і `gc_init` у `gc_wasm.c`; пакет видаляє P11.5.0.2 |
| `test/goroot/notapplicable.yaml` | причини описують збирач upstream |
| `dev/mise.lock`, `dev/pixi.lock` | `libgcc` — хибний збіг |

### Прийняття

- Збірка з прихованим від `pkg-config` `bdw-gc` — код 0; у `NEEDED` бінарника немає `libgc`. Збираються нативний `hello`, `gopacabra1to1` і wasip1.
- `gopacabra/fixtures.sh` з чистим `~/.cache/llgo` — 0 з 88.
- `go test ./...` (`-timeout 120m`): падають лише відомі `TestBuildDo_DepFingerprintAndVersion`, три `TestSIMDFMV*` (середовище), `TestWasmGCRootFrameLinksRuntimeChain` (P11.5.0.2) і `TestRunCmdPassesGoBuildFlags` (на цій машині `GOFLAGS=-mod=mod` у go env; з `GOENV=off` — ok). Рядки `FAIL` для `cl/_testgo/runextest/bar` і `internal/build/testdata/linkfail/missing` — вивід дочірніх збірок у тестах, що навмисно падають.
- Не перевірено: darwin, `GOROOT/test`.

## P11.5.0.2 Видалення `tinygogc` (2026-11-20)

Під обов'язковим `gopacabra` (а з ним `nogc`) `tinygogc` реально працював лише на js/wasm: `configureWasmGC` вмикав там `llgo.wasm.gc.linear` типово. На wasip1 з `nogc` він уже був вимкнений. Baremetal під `gopacabra` не збирався зовсім.

| Коміт | Що |
| --- | --- |
| `133ba32f7` | тест спершу: `TestNoTinyGoGC` — wasip1 (повна збірка), js/wasm (генерація: компонування потребує emcc), baremetal; пакет `tinygogc` не компілюється, явний `-tags llgo.wasm.gc.linear` відхиляється |
| `ae6346e6b` | видалено `tinygogc`, його C-файли (`gc_wasm.c`, `finalizer_wasm.c`, `wasi_gc_world.c`, `registry_wasi_threads.c`) і 53 файли рантайму з умовою, хибною за `llgo.wasm.gc.linear=false, nogc=true`; решту умов згорнуто. `configureWasmGC` і `applyWasmGCLinkFlags` замінено на `rejectWasmGCLinear`. Тести, що перевіряли лише збирач, видалено (перелік — у коміті) |
| `1bfd4739c` | `TestBaremetalRuntimeKeepsSchedulerFreeLifecycleStubs` перевіряє no-op `AddCleanupPtr` у `z_nogc.go` замість видаленого `cleanup_baremetal.go` |

Реалізація: 137 файлів, +76/−7966 рядків, 83 файли видалено.

**Baremetal.** Під `gopacabra` збірка будь-якої baremetal-цілі падала, бо baremetal має `GOOS=linux`:

- нативний алокатор `alloc_gopacabra.go` (pthread) обирався умовою `(linux || darwin)`, а файли з `AllocU` і `FreeDeferNode` для baremetal+nogc не обиралися (`interface conversion: types.Object is nil`). Тепер алокатор gopacabra має `!baremetal`, а baremetal бере `z_nogc.go` і `z_defer_nogc.go`;
- `currentAccountShard` — змінна TLS: компонувальник xtensa не знає її релокацію, на ARM без ОС немає `__aeabi_read_tp`. На baremetal це звичайна глобальна змінна (`account_shard_baremetal_gopacabra.go`): спільний шард лише зводить лічильники в один слот, додавання атомарні.

Ціль у тесті — `esp32`, а не `cortex-m4`: у `cortex-m4` немає libc, вона не компонується з жодним рантаймом (`build.sh` у `_demo/embed/targetsbuild` позначає такі цілі як «warned»).

**Цілі, де рантайм не працює.**

| Ціль | Причина | Задача |
| --- | --- | --- |
| `esp32c3` (RISC-V без розширення A) | облік gopacabra кличе `__atomic_fetch_add_4/8`, `__atomic_fetch_or_4`, `__atomic_fetch_sub_8`; `compiler-rt/atomic.c` для ESP не збирається (`rtlib/compiler_rt.go`) | P11.5.0.5: ціль видалено разом з ESP |

**Компіляторні GC roots лишились.** `cl/gcroot.go`, `ssa/gcroot.go` і `prog.EnableGCRoots` тепер ніколи не вмикаються. Код не видалено, щоб diff `cl`/`ssa` від upstream лишився малим.

**Тести wasip1 з `dev/test_wasm_wasi_threads.py`** (`wasm-wasi-gc-arena`, `wasm-wasi-threaded-gc`, `wasm-wasi-main-goexit-gc`, `wasm-finalizer-direct`, `wasm-root-sharing`) лишено: під `gopacabra` wasip1 і раніше йшов з `nogc`. Проба `llgo_wasi_worker_stack_bounds` у `wasm-wasi-threads` більше не включає `gc_wasm.c` і перевіряє лише, що стек потоку містить його `sp`.

### Що лишилось у `git grep -i 'tinygogc\|gc\.linear\|nonmoving'`

Поза `PLAN.md`, `PROGRESS.md`, `INTENT.md` і звітами:

| Файл | Причина |
| --- | --- |
| `README.md`, `CLAUDE.md` | речення «`tinygogc` видалено» |
| `internal/build/build.go` | `rejectWasmGCLinear` — повідомлення про видалений тег |
| `internal/build/gopacabra_test.go`, `testdata/notracinggc` | сам тест-сторож |
| `test/goroot/notapplicable.yaml` | причини описують збирач upstream |

### Прийняття

- `TestNoTinyGoGC` — ok для wasip1, js/wasm, esp32 і явного тегу; до реалізації — червоний.
- `go test ./internal/build/` з чистим `~/.cache/llgo` — лише відомі падіння: `TestBuildDo_DepFingerprintAndVersion`, три `TestSIMDFMV*` (darwin, середовище). `TestWasmGCRootFrameLinksRuntimeChain` видалено разом із збирачем. Нове падіння `TestBaremetalRuntimeKeepsSchedulerFreeLifecycleStubs` виправлено в `1bfd4739c`.
- `gopacabra/fixtures.sh` з чистим кешем — 1 з 88: `opstat`, відоме падіння середовища.
- Нативна фікстура `notracinggc` і `hello` — вивід як у `go run`.
- Не перевірено:
  - вивід під wasip1: wasmtime на стенді немає, node 20 не виконує модулі llgo (`try_table`, exnref);
  - js/emscripten: немає emcc;
  - запуск esp32 в емуляторі: немає qemu;
  - wasi-threads під Wasmer;
  - darwin.

## P11.5.0.5 Видалення підтримки ESP (2026-11-20)

Рішення розробника: ESP не потрібен. Тому задачу P11.5.0.5 змінено: було «рантайм на esp32c3 без `__atomic_*`», стало «видалити ESP повністю».

**Що видалено** (`4b40b5aa4`, 142 файли, −37 тис. рядків):

- цілі esp32, esp32c3, esp8266, інші Xtensa і `riscv32-esp`, а також похідні плати (40 файлів `targets/*.json`), ESP linker-скрипти, `targets/device/esp`;
- `internal/firmware/esp.go` (образ ESP); тепер `esp` → `bin` у `firmware.go` і `env.go`;
- newlib-esp32 (`compile/libc/newlibesp.go`); прапорці libc переїхали в `compile/libc/flags.go`;
- Xtensa:
  - cabi (`TypeInfoEsp32`);
  - `jmpbuf_xtensa_baremetal.go`;
  - збірка compiler-rt; riscv32 тепер завжди з `atomic.c` і `-march=rv32imac`;
- тестові дані cabi `esp32`/`esp32c3`, демо `_demo/embed/esp32*`, `testdata/esp32-serial`, скрипти запуску;
- `capture_esp.go` у `cl/_testlibc/callbackmethod`;
- набір емулятора: `TestRunEmbedEmulator`, `FilterEmulatorOutput`, вибір `embedded` у `run_go_tests.py`, крок `LLGO_EMBED_TESTS` у `go.yml`;
- кроки ESP qemu в CI: `install-esp-qemu.sh`, `setup-embed-deps`.

**Що змінено.** Тести, де ESP був просто прикладом вбудованої цілі, тепер беруть `rp2040`, `riscv-qemu`, `cortex-m-qemu` або `arduino`. Їх зміст не змінився. Є три винятки:

- `verify_export.sh` збирає `c-archive` для `riscv-qemu` замість образу esp32: жодна не-ESP baremetal-ціль не компонується (див. нижче);
- набори ESP32-C3 у `test/buildcache/test.sh` і `test/sizebuild` видалено без заміни, з тієї ж причини; sizebuild лишився для wasi;
- `TestNoTinyGoGC` перевіряє baremetal на `riscv-qemu` у `ModeGen` (`7ebebaee6`). У P11.5.0.2 там була компоновка esp32.

**Що лишилось і чому.**

| Залишок | Причина |
| --- | --- |
| ESP Clang (`getESPClangRoot`, `-force-espclang`, `download_esp_clang.sh`, `TestESPClang*`, розділ у `THIRD_PARTY_NOTICES.md`) | вбудований clang для всіх вбудованих цілей, не лише ESP |
| тег compiler-rt `xtensa_release_22.1.4_*` | назва релізу `goplus/compiler-rt`, з якого збираються всі вбудовані цілі |
| `internal/crosscompile/libc_test.go` | рядки виводу ESP Clang як дані тесту версії компілятора |
| `ssa/decl.go` | коментар про випадок, знайдений на ESP32-C3 |
| `internal/build/gopacabra_test.go` | `TestNoESPTargets`, сам тест-сторож |

**Не-ESP baremetal-цілі не компонуються.** Це було і до видалення, ESP-цілі лишались єдиними, що компонувалися:

- `cortex-m-qemu`: немає `Reset_Handler`;
- `riscv-qemu`: немає `srandom`, `stdout`, `vfprintf`;
- `rp2040`: немає `__flash_size`.

Якщо baremetal знову знадобиться, задача — полагодити одну з цих цілей.

**`-oimg`.** Опцію з upstream лишено, але тепер вона завжди дає помилку. `ConvertFormats` просить формат `<binary-format>-img`, а `makeFirmwareImage` знав лише `esp-img`; лишились `uf2` і `nrf-dfu`. Можлива наступна чистка.

### Прийняття

- `TestNoESPTargets` — ok; до реалізації — червоний.
- `go build ./...`, `gofmt`, `go vet ./internal/... ./cl/... ./ssa/... ./cmd/...` — чисто.
- `go test ./internal/... ./cl/ ./cmd/...` — 93 пакети ok. Падіння лише відомі: `TestBuildDo_DepFingerprintAndVersion`, `TestSIMDFMVLLVM`, `TestSIMDFMVAggregatesLLVM`, `TestSIMDMidwayFMVLLVM`, `TestRunCmdPassesGoBuildFlags` (`GOFLAGS` оточення).
- Python-тести `.github/scripts` — ok.
- Не перевірено: повний CI (кроки вбудованих цілей змінено); запуск будь-якої baremetal-цілі; darwin.

## P11.5.0.6 Видалення baremetal (2026-11-20)

Рішення розробника: cortex-m, riscv, rp2040 та решта baremetal не потрібні. Лишились лише WebAssembly-цілі: wasm, wasi, wasip1, emscripten, emscripten-memory64.

**Що видалено** (`c9ff172c7`, 1247 файлів, −99,7 тис. рядків):

- усі не-wasm цілі й плати в `targets/*.json`, linker-скрипти, `targets/device` (738 файлів);
- збірка picolibc і compiler-rt (`internal/crosscompile/compile`, `libc.go`);
- образи прошивки (`internal/firmware`: bin/hex/img/uf2/zip) і пакети `internal/gohex`, `internal/crc16`;
- прошивка і монітор порту: `internal/flash`, `internal/monitor`, команда `llgo monitor`, прапорці `-obin`/`-ohex`/`-oimg`/`-ouf2`/`-ozip`/`-port`/`-baudrate`; залежності `go.bug.st/serial`, `mattn/go-tty`, `creack/goselect`, `mattn/go-isatty`;
- baremetal-файли рантайму (setjmp для arm/avr/cortex-m/riscv, `stdio_baremetal.go`, `z_baremetal.go`, `proc_baremetal.go`, час, poll, unwind тощо) і умови збірки `baremetal` у решті файлів;
- патч `atomic_llgo_arm.go`, демо `_demo/embed`, workflow `targets.yml`, дія `setup-gdb`, `doc/Embedded_Cmd.md`, ліцензії видаленого коду в `LICENSES/`;
- `Config.Emulator`, `Config.OutFmts`, поля Bin/Hex/Img/Uf2/Zip в `OutFmtDetails`. Іменовані цілі завжди запускаються через емулятор цілі.

Тести, що перевіряли лише видалене (образи, `-port`, вибір baremetal-файлів рантайму, `llgo monitor`), видалено. Тести, де baremetal-ціль була прикладом іменованої цілі, тепер беруть `wasi`.

**Що лишилось і чому.**

| Залишок | Причина |
| --- | --- |
| ESP Clang (`getESPClangRoot`, `-force-espclang`, `TestESPClang*`) | потрібен для крос-компіляції під інші ОС |
| `-emulator` | прапорець з upstream, тепер no-op |
| `llgo debug -remote/-server/-load`, поля `Linker`, `LinkerArgs`, `ExtraFiles`, `GDB`, `TargetABI`, `ClangRoot` в `Export` | загальні механізми, не прив'язані до baremetal; мінімальний diff з upstream |
| шляхи `cabi`/`ssa` для riscv32, avr, thumb | загальні для LLVM-цілей; їх тести беруть імена цілей як рядки |
| імена цілей `rp2040`, `riscv-qemu` у тестах `cl` і `internal/build` | рядки без резолву цілі; тести перевіряють логіку для довільної іменованої цілі |
| неперейменовані `runtime_baremetal.go`, `signal_baremetal_llgo.go`, `local_context_baremetal.go` | файли й далі потрібні wasm; перейменування — зайвий diff |

**Уточнення до P11.5.0.5.** Фраза «не-ESP baremetal-цілі не компонуються» стосувалась перевірених цілей: `cortex-m-qemu`, `riscv-qemu` і `rp2040`. Інші цілі не перевірялись.

### Прийняття

- Тест спершу (`cd81fe952`): `TestNoBaremetalTargets` — у `internal/targets` лише WebAssembly-цілі. До реалізації — червоний, після — ok.
- `go build ./...`, `gofmt` (крім згенерованого `xgo_autogen.go`), `go vet ./cmd/... ./internal/... ./ssa/... ./cl/` — чисто.
- `go test ./internal/... ./cmd/... ./ssa/... ./cl/` з чистим кешем llgo: падіння лише відомі: `TestBuildDo_DepFingerprintAndVersion`, `TestSIMDFMVLLVM`, `TestSIMDFMVAggregatesLLVM`, `TestSIMDMidwayFMVLLVM`, `TestRunCmdPassesGoBuildFlags` (`GOFLAGS` оточення). Шість нових падінь першого прогону (тести видаленого: вибір baremetal-файлів рантайму, артефакти bin/hex/uf2, розширення `.elf`, `llgo monitor`) виправлено в коміті реалізації; `internal/build` для них і `cmd/llgo` повністю — ok.
- Не перевірено: модуль `runtime` окремо (`go test` у ньому не запускався), повний CI, darwin.

## P11.5.1 Неперекладна функція асемблера без посилань (2026-11-21)

**Проблема.** `plan9asm` не перекладає функцій з параметрами або результатами-структурами, `complex64/128` і `double` у `CALL`. Помилка однієї функції зупиняла збірку всього пакета: `modernc.org/libc/abi0_linux_amd64.s` має 1 875 ABI0-обгорток, і кілька неперекладних серед них блокували libc, навіть коли програма їх не кличе.

**Рішення** (лише `internal/plan9asm`, `internal/build`; зовнішній `github.com/xgo-dev/plan9asm` не змінено):

- `TranslateSourceModuleForPkgWithOptions`: якщо переклад файлу падає, `untranslatableFuncs` шукає функції, через які він падає. Спершу пробний переклад без жодної функції: він перевіряє, що файл розбирається, і збирає імена функцій. Потім файл перекладається з рештою функцій. Якщо помилка називає функцію, ця функція перевіряється окремо; якщо ні, набір ділиться навпіл, доки не лишиться функція, що не перекладається й сама. Знайдена функція виключається через `KeepFunc`, причина (текст помилки після імені) зберігається в `ModuleTranslation.Untranslated`. Файл перекладається без неперекладних функцій; якщо не перекладається жодна, лишається порожній модуль.
- Розбір усього файлу займає більшість часу спроби (≈50 мс на 585 КБ `abi0_linux_amd64.s`), а помилка називає одну функцію. Тому перед циклом кожна функція перекладається з короткого фрагмента: спільні рядки (усе до першого `TEXT`, `#include`, `#define`, `DATA`, `GLOBL`) плюс її текст від `TEXT` до наступного `TEXT` (`splitFuncs`). Функція, помилка якої називає її саму, — неперекладна; цикл перевіряє решту разом і доловлює те, що фрагменти пропустили. Файл з `#if…`/`#else`/`#undef` не ділиться: їхній зміст залежить від місця в файлі.
- Помилка, що не зводиться до окремих функцій, — як раніше: помилка розбору файлу, помилка без імені функції (наприклад, недійсний режим x87), помилка, яка зникає при перекладі функцій поодинці.
- Результат пошуку запам'ятовується за пакетом, файлом, ціллю, опціями і SHA-256 вмісту: той самий файл перекладається кілька разів за збірку.
- Неперекладна функція лишається оголошенням без тіла з Go-боку. Якщо на неї немає посилань, збірка проходить. Якщо є — компоновка падає; `linkObjFiles` зберігає вивід компоновника в `linkError`, а `executeInitialPackageLink`, де є пакети збірки, додає до помилки рядок для кожної неперекладної функції, названої у виводі:

  ```
  example.com/p9untranslated/asmlib.Sum: plan9asm could not translate …/asmlib_amd64.s: unsupported struct type struct{Lo uint64; Hi uint64}
  ```

  Причину обчислюють заново під час пояснення, а не беруть зі стадії компіляції: пакет з кешу llgo не перекладається.

**Прийняття.**

- Тест спершу (`5230fa362`): `internal/build/plan9asm_untranslated_test.go` — пакет з `.s`, де `Add` перекладається, а `Sum` має параметр-структуру. Програма, що кличе лише `Add`, збирається і друкує як `go run`; програма, що кличе `Sum`, — помилка з `asmlib.Sum` і `unsupported struct type`. На HEAD до реалізації обидва підтести червоні (переклад пакета падає), після — зелені.
- `internal/plan9asm/untranslated_test.go` (з комітом реалізації): звичайний файл, умовна компіляція, файл без жодної перекладної функції. Він зелений і до поділу на фрагменти; мутація — поділ без відмови на `#if` — дає червоний випадок `conditional` (F помилково неперекладна).
- `TestUnusedGlobalInitNativeInputs`: випадок `INVALID` в тілі функції тепер не «файл без перекладу», а неперекладна функція — вона не потрапляє в бінарник, тож відсікання глобалів дозволене (`prune: true`). Помилки розбору (`TEXT ·Read(SB` без дужки) лишились файловими — окремий випадок `invalid assembly`.
- `modernc.org/libc` v1.80.1 (пробна програма з `libc.Xabs`), чистий кеш llgo, linux/amd64: у `abi0_linux_amd64.s` неперекладні всі 1 875 функцій (1 799 — `GO_ARGS`, P11.5.2; 45 — `complex128`; 23 — `complex64`; 12 — параметр-структура), у `tls_linux_amd64.s` — 4. Збірка — код 0, бінарник друкує `3`, як `go run`. Час збірки: 261 с з пошуком лише через переклад усього файлу (3 750 спроб, 3 хв 1 с), 72 с з фрагментами.
- `go test ./internal/plan9asm/ ./internal/build/` — падіння лише відомі: `TestBuildDo_DepFingerprintAndVersion`, `TestSIMDFMVLLVM`, `TestSIMDFMVAggregatesLLVM`, `TestSIMDMidwayFMVLLVM`. `gofmt`, `go vet ./internal/plan9asm/ ./internal/build/` — чисто.
- Не перевірено: darwin, повний CI, збірка `modernc.org/sqlite` (її блокують P11.5.2–P11.5.4).

## P11.5.2 Макроси `funcdata.h` і ABI0-кадр amd64 (2026-11-21)

**Проблема.** `modernc.org/libc` пише ABI0-обгортки з `#include "funcdata.h"`, `GO_ARGS` і `NO_LOCAL_POINTERS`. `plan9asm` не знав цих макросів (`amd64: unsupported instruction GO_ARGS`), тож 1 799 функцій `abi0_linux_amd64.s` і 4 функції `tls_linux_amd64.s` не перекладались. Після визначення макросів відкрилась друга вада: на amd64 обгортка кладе аргументи в `0(SP)…` і кличе Go-функцію через `CALL ·f(SB)` без `<ABIInternal>`, тобто за ABI0. `plan9asm` давав локальний стек лише 386, а на amd64 `SP` був звичайним регістром з нулем, тож виклик отримував невизначені аргументи.

**Рішення** (рішення розробника: форк `plan9asm` і ABI0-кадр — одна задача):

- `internal/plan9asm/translate.go`: перед кожним файлом порожні `#define GO_ARGS`, `GO_RESULTS_INITIALIZED`, `NO_LOCAL_POINTERS`. `PCDATA`/`FUNCDATA` `plan9asm` уже пропускав.
- `github.com/xgo-dev/plan9asm` v0.6.2 скопійовано в `gopacabra/third_party/plan9asm` і підключено через `replace`; латки описано в його `GOPACABRA.md`:
  - amd64 отримує `%local_stack`, щонайменше `$frame` з `TEXT` байтів над початковим SP;
  - сигнатура оголошеної Go-функції несе розкладку її ABI0-кадру;
  - `CALL` без `<ABIInternal>` до функції з кадром читає аргументи з `0(SP)…`, кличе і пише результати назад; `<ABIInternal>` лишає передачу через регістри;
  - якщо SP ніде не змінюється, `off(SP)` — GEP у `%local_stack` замість `inttoptr`: LLVM розрізняє слоти кадру, і після `O2` кадр зникає — обгортка кличе функцію прямо з власними параметрами.
- Відоме обмеження: іменований псевдо-SP (`x-8(SP)`) розбирається як апаратний SP.

**Прийняття.**

- Тест спершу (`41f3ec48b`): `internal/plan9asm/funcdata_test.go` — переклад функції з `#include "funcdata.h"`, `GO_ARGS`, `NO_LOCAL_POINTERS` і викликом Go-функції; фікстура `_cmptest/asmfuncdata` (amd64 `.s`, Go-версія для інших архітектур).
- Тест спершу (`3e229344c`): `amd64_abi0_call_test.go` у форку — скаляр, два результати, рядок-аргумент, `<ABIInternal>`. Після `default<O2>` тіло — прямий виклик з `%arg0, %arg1`; до реалізації всі чотири випадки давали `unreachable`.
- Реалізація (`2669f5535`): усі тести форку (`GOFLAGS=-mod=mod go test .`) і `go test ./internal/plan9asm/` — зелені. `llgo cmptest ./_cmptest/asmfuncdata` з чистим кешем — вивід як `go run` (`42`, `-23`).
- `modernc.org/libc` v1.80.1, пробна програма з `libc.NewTLS` (`TLSAlloc` з `tls_linux_amd64.s`) і `libc.Xabs`, чистий кеш, linux/amd64: збірка — код 0, бінарник друкує `3`, як `go run`. Неперекладних функцій 183 замість 1 879: результат `float32` через `MOVL` у `+N(FP)` (83), `complex128` (45), `complex64` (23), `CALL *AX` (19), параметр-структура (12), виклик функції без Go-оголошення (`Y__libc_sigaction`, 1). У `tls_linux_amd64.s` — жодної.
- `go test ./internal/plan9asm/ ./internal/build/` — падіння лише відомі: `TestBuildDo_DepFingerprintAndVersion`, `TestSIMDFMVLLVM`, `TestSIMDFMVAggregatesLLVM`, `TestSIMDMidwayFMVLLVM` (і їхні внутрішні рядки `runextest`/`linkfail`). `gofmt`, `go vet ./internal/plan9asm/ ./internal/build/` і `go vet` форку — чисто.
- Не перевірено: darwin, повний CI, збірка `modernc.org/sqlite` (її блокують P11.5.3–P11.5.4).

## P11.5.3 Значення функції з однословного об'єкта (2026-11-21)

**Проблема.** ccgo кличе функцію за словом даних інтерфейсу (`__ccgo_fp`) так: `(*(*func(...))(unsafe.Pointer(&struct{ uintptr }{fp})))(...)`. У `gc` значення функції — одне слово, вказівник на funcval. У llgo воно займає два слова `{fn, ctx}`, тож llgo читав 16 байтів з 8-байтового об'єкта: `fn = fp` і сміття замість `ctx`. Виклик ішов за адресою коробки і падав з `nil pointer dereference`.

**Рішення.** `cl/compile.go` `wordFuncValueSource`: якщо `*x` має тип функції, а `x` — `Convert` з `unsafe.Pointer`, який своєю чергою — `Convert` вказівника на об'єкт розміру слова, то `cl` читає слово `w`, а потім `{fn, ctx}` за `w`. Саме так llgo кладе функцію в інтерфейс: слово даних указує на коробку `{fn, ctx}`. Правильна llgo-програма так не читає (це був вихід за межі об'єкта), тож для upstream зміна нейтральна.

Обмеження: нульове слово дає падіння на читанні, а не nil-функцію; ccgo перевіряє `uintptr` на нуль до виклику.

**Прийняття.**

- Тест спершу (`6a6dec78d`): `_cmptest/ccgofp` і `cl/_testgo/ccgofp` — статична функція і замикання через `__ccgo_fp`, читання через `&struct{ uintptr }{fp}` і `&fp` (`fp uintptr`). На HEAD обидва падали з `panic: runtime error: invalid memory address or nil pointer dereference`.
- Реалізація (`a58ebf288`): `llgo cmptest ./_cmptest/ccgofp` — вивід як `go run` (`5`, `9`, `42`, `56`); FileCheck у `cl/_testgo/ccgofp` — чотири пари `load ptr` → `load { ptr, ptr }` з цього слова.
- `go test -count=1 -timeout 90m ./cl/` з чистим кешем — `ok` (707 с), падінь немає. `gofmt`, `go vet ./cl/` — чисто.
- Не перевірено: darwin, повний CI, `modernc.org/sqlite` (блокує P11.5.4).
