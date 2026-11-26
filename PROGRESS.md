# Gopacabra — PROGRESS

Журнал сесій: дата, задача з `PLAN.md`, що зроблено, що лишилось.

Записи до фази 12 (2026-10-29 — 2026-11-24, P0.1 — P11.8) прибрано; вони в git-історії цього файлу (`git log -- PROGRESS.md`). Підсумки закритих фаз — `INTENT.md` («Рішення гейта …») і `gopacabra/REPORT-*.md`.

## 2026-11-24 — P12.1 План фази 12, P12.2 Коробка виклику через слово

- P12.1 (`4d06d4962`): `go build -gcflags=-m` для `modernc.org/sqlite/lib` дає 0 `moved to heap`, а коробки — `does not escape`. Отже, `gc` тримає на стеку всі місця `pinned` з P11.6. Уточнення до P11.6: головні локальні змінні (`_yy_reduce`, `_sqlite3VdbeExec`, `_sqlite3ExprCodeTarget`) передають `unsafe.Pointer(&x)` аргументом, а не через `uintptr`. Задачі P12.2–P12.6 записано в `PLAN.md`, правила — в `gopacabra/REPORT-PHASE12.md`.
- P12.2:
  - фікстура `gopacabra/bytes/ccgoaddr` і тест `TestGopacabraPinnedCcgoAddr` (`4764769c8`), спершу червоний: `wordcall live pinned delta 512`;
  - правило в `gopacabraLocalPun` (`8a87d9871`): ширший `*func`, який читають лише за формою P11.5.3, не виходить за межі коробки.
- Пастка: `*ssa.DebugRef` серед referrers перетворення. Перша версія правила відкидала коробку на ньому.
- Перевірки:
  - `go test ./cl/ -run Gopacabra` — `ok`;
  - `gopacabra/fixtures.sh` — 0 з 90 падінь;
  - `go vet ./cl/` — чисто;
  - фікстура під `-O0 -asan` — код 0.
- Сервіс, `gopacabrastat`, N = 10 000: виділень `pinned` на 32.6% менше (6.75 млн → 4.55 млн), усіх виділень — на 16.4%, live `pinned` — на 19.9%. Місць коробок у звіті більше немає.
- Не перевірено: сервіс під ASan з новим правилом, darwin.
- Наступне: P12.3 — аргумент параметра `unsafe.Pointer`, що не тікає.

## 2026-11-24 — P12.3 Аргумент параметра `unsafe.Pointer`, що не тікає

- Тест спершу (`c86eef4eb`): кейси `arg` і `argescapes` у фікстурі `gopacabra/bytes/ccgoaddr` за формами `_sqlite3FinishTrigger`, `_sqlite3BtreeBeginTrans`/`GetMeta` і рекурсії з `unsafe.Add`. До виправлення — 2304 байти `pinned`.
- Правило (`fa705211a`): `gopacabraDerefParams` — найбільша фіксована точка параметрів `unsafe.Pointer`, які лише розіменовують, у межах пакета. `gopacabraLocalPun` приймає `unsafe.Pointer(&x)` як аргумент такого параметра.
- Перевірки:
  - `go test ./cl/ -run Gopacabra` — `ok`;
  - `gopacabra/fixtures.sh` — 0 з 90 падінь;
  - `go vet ./cl/` — чисто;
  - фікстура під `-O0 -asan` — код 0;
  - сервіс під `-O0 -asan`: юніт-тести — 32 PASS; сценарій на 100 000 запитів — `errors=0`, без звітів.
- Сервіс, `gopacabrastat`, N = 10 000: виділень `pinned` 110 тис. (було 4.55 млн після P12.2), live `pinned` — 3.4 МБ (було 70.7 МБ), RSS стала — 101 МБ (було 172 МБ). Тепер live визначає `rc-local`: ~3 КБ на запит.
- Форми `uintptr(unsafe.Pointer(&x))` серед головних місць залишку `pinned` немає. Чи потрібна P12.4 для мети фази, вирішує розробник.
- Не перевірено: darwin.
- Наступне: P12.4 або одразу P12.5 — розклад `rc-local`.

## 2026-11-24 — P12.5 Залишок live, P12.5.1 Комірка в defer-замиканні

- P12.5, розклад на `fa705211a`: `heapdiff.py` між 10 000 і 30 000 запитів. Приріст `rc-local` — ~2.77 КБ на запит, `pinned` — 328 Б, `rc-shared` — 179 Б, `cyclic` — 70 Б. Головні місця — функції `modernc.org/libc` (`Xmemcpy`, `Xmemset`, `Xstrlen`, `UintptrMalloc`/`Free`) з формою ccgo `if __ccgo_strace { defer func() { trc(r) }() }`. Комірку `r` не звільняли, бо серед її користувачів є `*ssa.Defer`.
- P12.5.1:
  - тест спершу (`252dff622`): фікстури `gopacabra/bytes/deferclosure` і `gopacabra/asan/deferclosure`, тест `TestGopacabraDeferClosureCell`; до виправлення — `constdefer live rc-local delta 512 want 0`;
  - правило (`108ca546c`) у `gopacabraFreeAfter`: місце, яке тримає defer, звільняється після `RunDefers`, якщо алокація стоїть у блоці входу до першого defer (`deferEntrySite`); на шляху `panic` — не звільняється.
- Пастка: продовження після `RunDefers` у LLVM досяжне і з `setjmp`. Алокація поза блоком входу не домінує над ним, і LLVM verify падає («Instruction does not dominate all uses» у функціях рантайму з defer у циклі).
- Перевірки:
  - `go test -count=1 ./cl/ -run Gopacabra` — `ok`;
  - `go test ./gopacabra/census/` — `ok`;
  - `go vet ./cl/` і `gofmt -l` — чисто;
  - обидві фікстури під `-O0 -asan` — код 0;
  - сервіс під `-O0 -asan`: юніт-тести — 32 PASS; сценарій на 100 000 запитів — `errors=0`, без звітів;
  - `gopacabra/fixtures.sh` — 1 з 92 падінь, `accountshard`. Ця фікстура нестабільна й до виправлення: на 60 прогонах — 9 падінь без P12.5.1 і 5 з ним. Різниця — 136 байтів на потік M:N, що стартує між знімками (`thread_unix.go:64`: 8–10 потоків на прогін).
- Сервіс, `gopacabrastat`, N = 10 000:
  - live `rc-local` — 12.2 МБ замість 30.8 МБ (−60%);
  - RSS стала — 75 МБ замість 101 МБ;
  - `errors=0`.
- Залишок і задачі P12.5.2–P12.5.6 — у `gopacabra/REPORT-PHASE12.md` і `PLAN.md`. Найбільше місце — параметр `database/sql.convertAssignRows`, який захоплює замикання в полі `rows.cancel`.
- Не перевірено: darwin; розклад у байтах після P12.5.1.
- Наступне: P12.5.2 або P12.4.

## 2026-11-24 — P12.5.2 Параметр, захоплений замиканням у полі

- Тест спершу (`e7a8d847e`): фікстури `gopacabra/bytes/closurefield` і `gopacabra/asan/closurefield`, тест `TestGopacabraClosureFieldCell`; до виправлення — `field live rc-local delta 512 want 0`.
- Правило: замикання, що йде з функції, більше не забирає захоплену комірку на всіх шляхах. `census` повертає такі замикання в `Placement.Escapes`, а `gopacabraFreeAfter` не ставить `Dec` у блоках, досяжних з `MakeClosure`, і після нього в його блоці. `TestClosureLeavesWithCapturedCell` перевіряє новий контракт: на шляху замикання `Dec` немає.
- Форма `return` не покрита: результат блокує `exitSites` на всіх шляхах. У фікстурі для неї перевіряються лише значення.
- Перевірки:
  - `go test -count=1 ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `go test ./gopacabra/census/` — `ok`;
  - `go vet ./cl/ ./gopacabra/census/` і `gofmt -l` — чисто;
  - `gopacabra/fixtures.sh` — 0 з 93 падінь (ASan-фікстура входить);
  - сервіс під `-O0 -asan`: юніт-тести — 32 PASS; сценарій на 100 000 запитів — `errors=0`, без звітів.
- Сервіс, `gopacabrastat`, N = 10 000: `convert.go:247` — 134 572 `dec_accounted` на 134 572 виділення; live `rc-local` — 11.1 МБ замість 12.2 МБ; `errors=0`.
- Не перевірено: darwin; RSS (один прогін, порівняння з P12.5.1 не робилось).
- Наступне: P12.5.3 (`GoString`) або P12.5.4 (`encoding/json/v2`).


## 2026-11-24 — P12.5.6 Розклад у байтах і буфер подій `netpoll` (P12.5.6.1)

- Розклад після P12.5.2 (`heapsym.py`, сервіс, N = 10 000): live 6488 Б на запит. Найбільше — `runtime.netpoll` (~2.9 КБ на запит) і `reflect.unpackEface` (~280 Б). Обидва місця — `unplaced`: `prepareGopacabraFree` розставляє лише функції оригінального пакета, а не оверлея (`patch.Alt`). Лічильники `gopacabra live` таких місць не бачать.
- P12.5.3 зводиться до P12.5.5: рядки `GoString` живуть у зрізах об'єкта `rows`, який сам лишається живим.
- Тест спершу (`43ce4e96a`): фікстура `gopacabra/bytes/netpoll` (1000 обмінів через loopback), тест `TestGopacabraNetpollEvents` під `gopacabrastat` читає рядки `gopacabra site` файлів `netpoll_*`. Червоний: `netpoll_epoll_gopacabramn_llgo.go:81 alloc: 4051`.
- Виправлення (`9f1675fed`): буфери, які бачить ядро (`events`, `ev`, `ts`), у версіях epoll і kqueue виділяються на C-стеку через `c.Alloca`.
- Перевірки:
  - `go test -count=1 ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `gopacabra/fixtures.sh` — 0 з 94 падінь;
  - сервіс під `-O0 -asan`: сценарій на 100 000 запитів — `errors=0`, без звітів.
- Сервіс, `gopacabrastat,gopacabraheap`, N = 10 000: live за профілем — 3524 Б на запит замість 6488; місць `netpoll` у профілі немає; `errors=0`.
- Нова задача P12.5.6.2 — census для функцій оверлея (`reflect.unpackEface`: 190 085 виділень без звільнення на 10 000 запитів).
- Не перевірено: darwin (версія kqueue змінена, але не зібрана).
- Наступне: P12.5.6.2 або P12.5.5.

## 2026-11-24 — P12.5.6.2 Census для функцій оверлея

- Тест спершу (`c5c18fe58`): фікстура `gopacabra/bytes/overlaysite` (4000 викликів `reflect.ValueOf`), тест `TestGopacabraOverlaySites` під `gopacabrastat`.
- Виправлення (`c71de8c90`): `prepareGopacabraFree` розставляє функції пакета разом із функціями його оверлея (`patch.Alt`). `ssaPackageFuncs` приймає кілька пакетів і не повторює методи спільної області типів.
- Оверлей самого `runtime` census не розставляє: пули й звільнення рантайму потрапили б у `Live` і `Freed` програми. Тест його рядки пропускає; ця умова додана в тест разом із виправленням, після коміту тесту.
- Перевірки:
  - `go test -count=1 ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `go test ./gopacabra/census/` — `ok`; `go vet ./cl/ ./gopacabra/census/` і `gofmt -l cl/` — чисто;
  - `gopacabra/fixtures.sh` — 0 з 95 падінь;
  - сервіс під `-O0 -asan`: юніт-тести — `ok`; сценарій на 100 000 запитів — `errors=0`, без звітів.
- Сервіс, `gopacabrastat,gopacabraheap`, N = 10 000, один прогін: live за профілем — 3247 Б на запит замість 3524; `reflect.unpackEface` у профілі немає; `errors=0`.
- Лишились `unplaced` поза оверлеєм `runtime`: `fiber/v3` `helpers.go:995` і `:1078`, по ~4000 виділень на 10 000 запитів. Причина не з'ясована, задачі немає.
- Не перевірено: darwin; повний `go test ./cl/` і `./internal/build/`.
- Наступне: P12.5.4, P12.5.5 або P12.4; далі гейт P12.6.

## 2026-11-24 — P12.5.5.1 Комірка змінної-вказівника на циклічний тип

- P12.5.5 поділено на три задачі за причинами: комірки `cyclic: closure` (P12.5.5.1), `Rows` (P12.5.5.2), `rows` драйвера sqlite (P12.5.5.3).
- Тести спершу: `81ee2ede7` — фікстура `gopacabra/bytes/withlock`, тест `TestGopacabraWithLockCell` (червоний: `live cyclic delta 1024 want 0`); `TestPointerCellIsNotCyclicNode` у census (червоний: `cyclic: closure`).
- Правило (`5118c22e9`): `category` для `*ssa.Alloc` змінної-вказівника шукає в графі типів `**T`, а не `*T`. Комірка `*DB` стає `rc-local: closure` і звільняється за правилом P12.5.2. Комірка лишається `cyclic`, якщо тип тримає `**T`.
- Нова ASan-фікстура `gopacabra/asan/withlock`: замикання в полі, з `return` і в горутині викликаються після повернення функції.
- Перевірки:
  - `go test -count=1 ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `go test ./gopacabra/census/...` — `ok`; `go vet ./cl/ ./gopacabra/census/` і `gofmt -l` — чисто;
  - `gopacabra/fixtures.sh` — 0 з 97 падінь;
  - сервіс під `-O0 -asan`: юніт-тести — `ok`; сценарій на 100 000 запитів — `errors=0`, без звітів.
- Сервіс, `gopacabrastat`, N = 10 000, один прогін: `sql.go:1742` і `:1776` — `dec_accounted` на кожне виділення; live `cyclic` — 784 520 Б замість 993 544; `errors=0`.
- Не перевірено: darwin; повний `go test ./cl/` і `./internal/build/`; `GOROOT/test`; профіль купи після зміни.
- Наступне: P12.5.5.2 або P12.5.5.3 (разом ~330 Б на запит), P12.5.4.

## 2026-11-24 — Розбір залишку після P12.5.5.1 (без коду)

- `Rows` (`sql.go:1800`, P12.5.5.2): наявні правила його не покривають. `sharedSites` працює в межах функції місця, а `Rows` іде через три функції й комірку замикання, `go` стоїть у його методі. Записано в `PLAN.md` і `gopacabra/REPORT-PHASE12.md`; потрібне рішення розробника про нове правило.
- Нова задача P12.5.7: аргументи `Scan` у коді сервісу (`main.go:195`/`:196`) — `rc-local shared`, по 39 980 виділень без звільнення на 10 000 запитів, ~310 Б на запит. Причину позначки не перевірено.
- `GOROOT/test` після P12.5.5.1 не запускався: на диску 4.4 ГБ, прогін лишає гігабайти кешу.
- Наступне: P12.5.7 (спершу причина `shared`), P12.5.4; P12.5.5.2 і P12.5.5.3 — після рішення про правило.

## 2026-11-24 — P12.5.7 Причина `shared` в аргументів `Scan` (без коду)

- Метод: тимчасове трасування `markUnknown` у census (не закомічене). Програма з фіктивним драйвером `database/sql/driver` збиралась через `llgo run`, а її зведення перевірялись `PublishProgram` над `go/packages`.
- `dest` у `(*sql.Rows).Scan` має `Unknown` з п'яти незалежних причин. Це `reflect.Indirect` → `(Value).Elem` → `verifyNotInHeapPtr` (функція без тіла), `Value.Set`/`SetZero`, методи `reflect.Type`, `fmt.Errorf(… %T …, dest)` і `driver.RowsColumnScanner.ScanColumn`. Якщо прибрати одну причину, параметр лишиться `Unknown` через інші.
- `gc` теж переносить ці змінні в купу: `moved to heap: it` для `main.go:108`, `:125`, `:141`, `:195`, `:212` сервісу `rest`.
- Знайдено ризик: зведення функцій `reflect` береться з тіла `std`, а компілюється оверлей. Чи веде це до хибного звільнення, не перевірено.
- У `PLAN.md` додано P12.5.7.1–P12.5.7.4. Рішення, чи брати їх у фазу 12, — за розробником. P12.5.7 лишається відкритою.
- Не перевірено: darwin; поведінку оверлея `reflect` під census для інших викликів.
- Наступне: рішення про P12.5.7.x і P12.5.5.2; P12.5.4 від цього рішення не залежить.

## 2026-11-25 — P12.5.4 `encoding/json/v2`, `arshal_default.go:231`

- Причина: правило зведення для `append` клало токени `src` у вміст результату й для байтів. `appendEscapedASCII` повертає свій аргумент, тож `jsonwire.AppendQuote` отримував `src` `Alias [0]`. Результат іде в `xe.Buf`, і `[]byte(s)` у `makeStringArshaler` позначався `rc-local call`.
- Тести спершу: `11128d8cb` — `TestAppendCopiesElements` у census (червоний для `Quote`/`QuoteString`), фікстура `gopacabra/bytes/jsonquote` і `TestGopacabraJSONQuote` (червоний: 1000 виділень, 0 звільнень).
- Правило (`1995e937a`): вміст другого аргументу `append` переходить у результат, лише коли елемент містить вказівники (`containsRef`); зріз з параметром типу — як раніше.
- Перевірки:
  - `go test -count=1 ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `go test ./gopacabra/census/...` — `ok`; `go vet ./gopacabra/census/` і `gofmt -l` — чисто;
  - `gopacabra/fixtures.sh` — 0 з 98 падінь;
  - сервіс під `-O0 -asan`, сценарій на 100 000 запитів — `errors=0`, без звітів.
- Сервіс, `gopacabrastat`, N = 10 000, один прогін: `:231` — `static string`, звільнення на кожне виділення; live `rc-local` 10 780 771 Б замість 11 076 831; `errors=0`.
- Не перевірено: darwin; юніт-тести сервісу під ASan; повний `go test ./cl/` і `./internal/build/`; `GOROOT/test`; профіль купи після зміни.
- Наступне: рішення розробника про P12.5.5.2 і P12.5.7.x; далі гейт P12.6.

## 2026-11-25 — P12.5.7.1 Зведення `reflect` з тіла, що компілюється

- Причина: `PublishProgram` і зведення на місці виклику брали тіло функції `std`, хоча cl компілює замість неї функцію оверлея (`patch.Alt`, `runtime/internal/lib/...`). Позичання за зведенням `std` могло звільнити аргумент, який оверлей утримує.
- Тест спершу (`d70c06825`): `TestOverlaySummary` у census — функція оверлея утримує параметр, її двійник у `std` позичає (червоний: зведення `std`). `PublishProgram` отримує оверлеї збірки (`ctx.patches` в `internal/build`).
- Реалізація (`71ae0f4b2`): `overlayTwins` зіставляє функцію чи метод `std` з функцією оверлея за іменем і сигнатурою; `compiled` підставляє двійника в `summaryOf`, `calledFns`, `callTargets`, `freshCall`, `InteriorBase`, `builderCall` і `goOperands`. `PublishProgram` скидає зведення й свіжість попередньої публікації: інакше вони просочувались у наступний прогін з іншими оверлеями.
- Перевірки:
  - `go test ./gopacabra/census/...` — `ok`; `go vet` і `gofmt -l` — чисто;
  - `go test -count=1 -timeout 90m ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `gopacabra/fixtures.sh` — 0 з 98 падінь;
  - сервіс під `-O0 -asan`, сценарій `-c 64 -n 100000` — `errors=0`, stderr порожній.
- Сервіс, `gopacabrastat`, N = 10 000, по два прогони до й після з чергуванням: live `pinned` 3.43 МБ → 3.92 МБ (+~495 КБ, ≈ +50 Б на запит); `rc-local`, `rc-shared`, `cyclic` — у межах розкиду. Нові місця `pinned unsafe` — коробки аргументів `libc.VaList` у sqlite (`sqlite_g_…7f00.go:113` — 18 414 виділень та інші): зведення оверлейного `reflect.TypeOf` (`unsafe.Pointer(&i)`, `toType`) дає параметру втечу, а параметр `args` у `VaList` — `Pinned`. Раніше зведення `TypeOf` з `std` позичало.
- Не перевірено: darwin; повний `go test ./cl/` і `./internal/build/`; `GOROOT/test`; юніт-тести сервісу під ASan; точний ланцюг втечі в оверлейному `TypeOf` (дампу зведень немає).
- Наступне: рішення розробника — прийняти регресію `pinned`, взяти P12.5.7.2 (`reflect.Value` і `Type` як позичання) чи окреме правило для `reflect.TypeOf`.

## 2026-11-25 — P12.5.7.2 `reflect.Value` як позичання

- Причина: після P12.5.7.1 зведення оверлейного `reflect` давали параметрам `TypeOf`, `Set` і `SetZero` утечу. На те було три причини: дескриптор `*abi.Type`, прочитаний з параметра, переносив його токени у виклики; читання комірки `x` у `Set` бачило пізніший запис результату `assignTo`; функції без тіла `typedmemmove`, `typedmemclr`, `ifaceE2I` мали зведення «невідомо».
- Тест спершу (`458811cea`): `TestReflectBorrow` (мініатюри оверлея з шляхами `runtime/abi` і `runtime/internal/lib/reflect`, з `GlobalDebug` і без; контролі на аліас і утечу); фікстура `gopacabra/asan/reflectset`.
- Реалізація (`effec795b`):
  - завантаження `*abi.Type` з пам'яті й `Field` цього типу токенів не несуть (`typeDescriptor`; інваріант оверлея — дескриптори статичні або в центральній мапі);
  - читання з локальної комірки, яку лише пишуть і читають за адресою, бачить лише записи, що досягають його в CFG (`cellLoads`, `blocksAfter`);
  - ручні зведення функцій оверлея без тіла (`runtimeSummaries`).
- Перевірки:
  - `go test ./gopacabra/census/` — `ok`; `go vet` і `gofmt -l` — чисто;
  - `go test -count=1 -timeout 90m ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `gopacabra/fixtures.sh` — 0 з 99 падінь;
  - сервіс під `-O0 -asan`, сценарій `-c 64 -n 100000` — `errors=0`, stderr порожній, код 0.
- Сервіс, `gopacabrastat`, N = 10 000, по два прогони до й після з чергуванням: live `pinned` 3.91 МБ → 3.43 МБ, тобто регресію P12.5.7.1 знято. Зникли 19 місць `pinned` sqlite (коробки `libc.VaList` та інші) і `gofiber/schema` `decoder.go:347`; нових немає. Місця `Scan` (`main.go:195`/`:196`) без змін.
- Не перевірено: darwin; повний `go test ./cl/` і `./internal/build/`; `GOROOT/test`; юніт-тести сервісу під ASan; час компіляції census (`cellLoads` рахується на кожен виклик `summarize`).
- Знахідка: `instrsAfter` і `usedAfter` пропускають інструкції власного блоку, досяжні зворотним ребром циклу. Це можлива ненадійність, не перевірена.
- Наступне: P12.5.7.3 (друк `fmt`) або P12.5.7.4 (цілі `ScanColumn`) — без них місця `Scan` лишаються `shared`; окремо — перевірка знахідки про `instrsAfter`.

## 2026-11-26 — P12.5.7.3 Друк `fmt`

- Причина: аргументи `fmt.Sprintf`/`fmt.Errorf` у зведенні викликача ставали `Unknown`, бо census не читає тіла `fmt`. Серед іншого, через `fmt.Errorf(… %T …, dest)` у `(*sql.Rows).Scan`.
- Тест спершу:
  - `955ab40aa`: `TestFmtBorrow` у census і `TestGopacabraFmtErrorf` у cl з фікстурою `gopacabra/bytes/fmterrorf`;
  - `d65c60b7e`: `TestCommaOkKeepsAlias`, обидва червоні до змін.
- Реалізація:
  - `83e7ab854`: `fmtCalled` (`gopacabra/census/fmtprint.go`). За константного формату і зрізу, зібраного на місці виклику, аргументи `%T`, `*` і базового типу без методів позичаються. Решта, включно з `%w`, лишається `Unknown` — це відхилення від плану (`Holds`), бо fmt викликає `Error()`.
  - `b0da318ca`: правило fmt виявило, що `Extract` кортежу comma-ok (`x.(T)`, `m[k]`, `<-ch`, `Next`) губив аліас параметра. Fiber `App.Get/Post/Add` почали позичати обробник, і сервіс падав nil deref на кожному запиті до бази. Тепер елемент кортежу, що не є викликом, бере токени самого кортежу.
- Перевірки:
  - `go test ./gopacabra/census/...` — `ok`; `go vet` і `gofmt -l` — чисто;
  - `go test -count=1 -timeout 90m ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `gopacabra/fixtures.sh` — 0 зі 100 падінь;
  - сервіс під `-O0 -asan`, сценарій `-c 64 -n 100000` — `errors=0`, stderr порожній, код 0.
- Сервіс, `gopacabrastat`, N = 10 000, по два прогони з чергуванням: `errors=0`; live `pinned`, `rc-local`, `rc-shared`, `cyclic` — у межах розкиду. Місця `Scan` (`main.go:195`/`:196`) без змін.
- Не перевірено:
  - darwin;
  - повний `go test ./cl/` і `./internal/build/`;
  - `GOROOT/test`;
  - юніт-тести сервісу під ASan;
  - дієслово `*` тестом не покрите;
  - `Fprintf`/`Printf`/`Sprint*` і переданий далі `args...` не покриті.
- Наступне: P12.5.7.4 (цілі `RowsColumnScanner.ScanColumn`) — без нього місця `Scan` лишаються `shared`.

## 2026-11-26 — P12.5.7.4 Цілі `RowsColumnScanner.ScanColumn`

- Причина: `dest` у `(*sql.Rows).scanLocked` ставав `Unknown` на виклику `ScanColumn` через `driver.RowsColumnScanner`, цілі якого census не знав.
- Тест спершу (`9cd57a128`): `TestNoImplementerBorrows` (census) і `TestGopacabraNoImplementer` (cl) з фікстурою `gopacabra/bytes/noimpl`. Census-тест на цьому коміті червоний; cl-тест до реалізації не перевірено.
- Реалізація (`259f1be1a`): `PublishProgram` збирає імена методів, оголошених у програмі (`methodNames`). Метод інтерфейсу без жодної реалізації дає `callTargets` відомий порожній набір цілей, а `mergedParam` — позичання.
- Перевірки:
  - `go test ./gopacabra/census/...` — `ok`; `go vet` і `gofmt -l` — чисто;
  - `go test -count=1 -timeout 90m ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `gopacabra/fixtures.sh` — 0 зі 101 падіння.
- Сервіс, `gopacabrastat`, N = 10 000, по два прогони з чергуванням: `errors=0`; live у межах розкиду. Місця `Scan` (`main.go:195`/`:196` та інші) без змін.
- Причина відсутності ефекту (тимчасове трасування `markUnknown`, не закомічене): `dest` тепер стає `Unknown` у `reflect.ValueOf(dest)` і `invoke AssignableTo` у `convertAssignRows`. Усередині оверлейний `unpackEface` має тіло зі `std` (`/usr/local/go/src/reflect/value.go:164`) з дескриптором `internal/abi.Type` зі `std`, якого правило P12.5.7.2 не знає. `Value.Elem`/`Set`/`Type` теж зводяться з тіла `std`.
- Не перевірено:
  - darwin;
  - повний `go test ./cl/` і `./internal/build/`;
  - `GOROOT/test`;
  - сервіс під `-O0 -asan` (правило лише додає позичання недосяжному виклику, а місця сервісу не змінились);
  - чому в збірці сервісу `reflect` зводиться з тіла `std`, а на фікстурах P12.5.7.1/P12.5.7.2 — ні.
- Наступне: рішення розробника — окрема задача «`reflect` у збірці сервісу з тіла, що компілюється» (фікстура з `sql.Rows.Scan` і фіктивним драйвером), або гейт P12.6 з відкритою P12.5.7.

## 2026-11-26 — P-NEW-03.6 Замір покинутих масивів росту

- Тест спершу (`f14e3b9bb`): фікстура `gopacabra/growstat` (ріст зрізу, ріст мапи без `range` і з ним, власницький `append` у межах ємності) і `TestGopacabraStatOwnedAppend`. До зміни обидва червоні: `slice grow_slice delta 0 want 11`.
- Реалізація (`4373887ba`): дев'ять рядків `gopacabra ops` під `gopacabrastat`:
  - `grow_slice*` — `GrowSliceAligned` зі старим `cap > 0`;
  - `map_grow_done*` і `map_grow_done_iter*` — `advanceEvacuationMark`, окремо для `oldIterator`;
  - `map_old_overflow` — `evacuate*`, до перенесення бакета;
  - `owned_append_in_cap*` — `OwnedAppendCapStat`, який під `gopacabrastat` кличе `emitOwnedAppend`.
- Без `gopacabrastat` гачки — порожні заглушки (`growstat_off.go`), `emitOwnedAppend` кличе `OwnedAppendCap`, як раніше.
- `b1bb76e95`: `gopacabra/opsite` пропускає нові рядки, бо вони не мають місця компілятора. `heapsym.py -via REGEX` лишає тільки стеки з потрібним кадром.
- Числа (`gopacabra/REPORT-STAGEC.md`, «Покинуті масиви росту»; сирі дані — `bench/stagec/growstat/`):
  - regex-redux — 881 МБ старих масивів зрізу, 64% пікової RSS; головний стек — `regexp.(*Regexp).ReplaceAll`;
  - k-nucleotide — 9.0 МБ старих таблиць мапи, 5.7% RSS;
  - решта корпусу — кілобайти;
  - `oldIterator` на корпусі — 0;
  - `fiberdb`, 259 812 запитів — ~0.2 Б на запит з обох подій;
  - подія (3): корпус — 0; стенд — 6 викликів, 0 байтів.
- Перевірки:
  - `go test -count=1 ./cl/ -run Gopacabra` з чистим кешем — `ok`;
  - `gopacabra/fixtures.sh` — 1 зі 102 (`opsite`), після `b1bb76e95` `opsite-ok`;
  - `go vet` рантайму з трьома наборами тегів — лише відомі попередження.
- Не перевірено:
  - darwin;
  - повний `go test ./cl/`;
  - `GOROOT/test` (рантайм без `gopacabrastat` змінено лише викликами порожніх заглушок);
  - чи `replaceAll` regex-redux підпадає під умову (а) P-NEW-03.8.
- Наступне: P-NEW-03.7 (стара таблиця мапи); рішення розробника про P-NEW-03.9 — число мале.
