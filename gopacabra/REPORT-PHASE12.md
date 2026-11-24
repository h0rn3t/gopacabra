# Фаза 12 — витік сервісу

## P12.1 Місця `pinned` під escape-аналізом `gc` (2026-11-24)

Команда, `go 1.27`, `linux/amd64`, модуль `gopacabra/stand/rest`:

```bash
CGO_ENABLED=0 go build -a -gcflags='modernc.org/sqlite/lib=-m' -o /dev/null .
```

Вихід для `modernc.org/sqlite@v1.62.0/lib` — 51 274 рядки. `moved to heap` трапляється 0 разів, `does not escape` — 5 054 рази. Отже, у `gc` кожна локальна змінна цього пакета лежить на стеку. Мовчання `-m` про змінну означає стек.

| Місце P11.6 | Приріст (P11.6) | `gc -m` | Форма адреси |
| --- | ---: | --- | --- |
| `sqlite_g_0000000000000900.go:27603`, `_yy_reduce` | 10 965 900 | стек (рядка немає) | `unsafe.Pointer(&x)` — аргумент `_sqlite3SrcListAppendFromTerm`, `_sqlite3FinishTrigger` |
| `sqlite_g_0000000000000900.go:18170`, `_sqlite3VdbeExec` | 7 161 600 | стек | `unsafe.Pointer(&iMeta)` — аргумент `_sqlite3BtreeBeginTrans`, `_sqlite3BtreeGetMeta` |
| `sqlite_g_0000000000000900.go:27605`, `_yy_reduce` | 3 655 300 | стек | те саме |
| `sqlite_g_0000000000000900.go:18175`, `_sqlite3VdbeExec` | 2 685 600 | стек | `unsafe.Pointer(&res1)` тощо — аргумент |
| `sqlite_g_000000000003fd6b.go:16144`, `_sqlite3Parser` | 1 504 884 | стек | `*(*TToken)(unsafe.Pointer(&yyminorunion)) = ...` і аргумент `_yy_destructor` |
| `sqlite_g_0000000000000900.go:12337`, `_sqlite3ExprCodeTarget` | 1 490 820 | стек | `unsafe.Pointer(&regFree1)` — аргумент |
| `sqlite.go:11434`, `:11459` (`FxMutexEnter`/`Leave`) | по 5 898 137 | `&struct { uintptr }{...} does not escape` | коробка виклику через слово |
| `sqlite_g_000000000003fd6b.go:19678` (`FxExprCallback`) | 1 947 084 | `&struct { uintptr }{...} does not escape` | коробка виклику через слово |
| `sqlite_g_0000000000018948.go:3087`, `_tokenExpr` | частина 2% | стек (`t` не переміщено) | `unsafe.Pointer(&t)` — аргумент `_sqlite3RenameTokenMap` |

**Уточнення до P11.6.** Звіт P11.6 описав локальні змінні як «адреса лише в `uintptr`». Насправді головні місця передають `unsafe.Pointer(&x)` аргументом у параметр типу `unsafe.Pointer` функції того самого пакета. Викликана функція лише розіменовує цей параметр: `*(*uintptr)(pAll)`, `unsafe.Add(pAll, 8)`, `*(*int32)(pReg) = r`. Форма `uintptr(unsafe.Pointer(&V))` у пакеті частіша (737 входжень), але серед головних місць P11.6 її немає.

Частота форм адреси локальної змінної в `modernc.org/sqlite@v1.62.0/lib`:

| Форма | Входжень |
| --- | ---: |
| `uintptr(unsafe.Pointer(&V))` | 737 |
| `*(*uintptr)(unsafe.Pointer(&V))` | 298 |
| `(uintptr(unsafe.Pointer(&V))`, у виразі | 96 |
| `...)(unsafe.Pointer(&V))`, аргумент | 73 |
| `__ccgo_ptr(uintptr(...))` | 65 |
| `unsafe.Add(unsafe.Pointer(&V), ...)` | 18 |

**Правила компілятора.** Правило census для `unsafe.Pointer` не змінюється: адреса, що дійшла до `unsafe.Pointer` і лишилась у купі, — `pinned`. Правила нижче знімають `Heap` з `*ssa.Alloc` до census, як P11.0.1 (`gopacabraStackPuns`, `cl/gopacabra_free.go`). Тоді місце стає `stack`.

1. **Коробка виклику через слово** (P12.2). `*(*func(...))(unsafe.Pointer(&obj))`, де `obj` розміру слова. Кодоген P11.5.3 (`wordFuncValueSource`) читає лише слово `obj`, а пару `{fn, ctx}` — за ним. Перевірка розміру в `gopacabraLocalPun` порівнює 16 байтів `func` з 8 байтами коробки, тому коробку відкидає. Правило: перетворення в `*func`, що має лише такі читання, не читає за межами `obj`.
2. **Аргумент параметра `unsafe.Pointer`, що не тікає** (P12.3). `f(..., unsafe.Pointer(&x), ...)`, де `f` — функція того самого пакета, а її параметр лише розіменовують: перетворення в `*T` з читанням і записом, `unsafe.Add` з тим самим використанням, передача далі в такий самий параметр. Перевіряється фіксованою точкою в межах пакета. Виклик функції іншого пакета, через значення чи інтерфейс правило не покриває. Межі об'єкта всередині викликаної не перевіряються, як і в `gc`: вихід за межі — помилка програми на C.
3. **Адреса лише в `uintptr`** (P12.4). `uintptr(unsafe.Pointer(&x))`, яке далі не перетворюють назад у вказівник у тій самій функції. `gc` вважає таке перетворення кінцем втечі. Стек llgo не рухається, і стек fiber M:N теж. Тому адреса лишається дійсною до виходу з функції, як у `gc`. Правило змінює лише фізичне місце об'єкта, і його потрібно перевірити під ASan на сервісі.

## P12.2 Коробка виклику через слово на стеку (2026-11-24)

Правило — у `gopacabraLocalPun` (`cl/gopacabra_free.go`). Перетворення адреси в `*T`, де `T` ширший за об'єкт, тепер приймається, якщо кожне читання через `*T` має форму P11.5.3 (`wordFuncValueSource`). Тоді кодоген читає лише слово об'єкта. Решта перевірок P11.0.1 не змінилась. Коробка, чия адреса йде в глобальну змінну, лишається `pinned`: це контрольний кейс фікстури `gopacabra/bytes/ccgoaddr`.

Сервіс `gopacabra/stand/rest`, збірка `-tags gopacabrastat`, сценарій P11.6 (`-c 64`, N = 10 000), один прогін до (`c8d43a882`) і після (`8a87d9871`):

| Лічильник | До | Після | Зміна |
| --- | ---: | ---: | ---: |
| `gopacabra ops alloc` | 13 843 860 | 11 566 565 | −16.4% |
| `note_alloc_pinned` | 6 751 557 | 4 551 451 | −32.6% |
| `note_alloc_rc-local` | 3 326 882 | 3 284 655 | −1.3% |
| `gopacabra live pinned`, байти | 88 263 290 | 70 661 606 | −19.9% |

Місця `sqlite.go:11434`, `:11459` і `sqlite_g_000000000003fd6b.go:19678` у рядках `gopacabra site` більше не з'являються. Прогон — `errors=0`, код виходу 0.

Залишок `pinned` — 455 виділень на запит. Це локальні змінні, чию адресу передають аргументом у параметр `unsafe.Pointer` (P12.3).

Не перевірено: сервіс під `-O0 -asan` з цим правилом (фікстура під ASan — код 0), darwin.

## P12.3 Аргумент параметра `unsafe.Pointer`, що не тікає (2026-11-24)

Правило — `gopacabraDerefParams` і `gopacabraDerefUse` у `cl/gopacabra_free.go`. До census обчислюється множина параметрів `unsafe.Pointer` функцій пакета, які лише розіменовують:
- перетворення в `*T` з читанням і записом;
- `unsafe.Add` з тим самим використанням;
- порівняння `==`/`!=`;
- аргумент параметра з цієї ж множини.

Множина — найбільша фіксована точка: спершу всі параметри, потім викидаються ті, що мають інше використання, доки множина не стабілізується. `gopacabraLocalPun` приймає `unsafe.Pointer(&x)`, який стає аргументом статично відомого виклику в такий параметр або `unsafe.Add` з тим самим використанням.

Сервіс, `-tags gopacabrastat`, сценарій P11.6 (`-c 64`, N = 10 000), один прогін на кожному коміті:

| Лічильник | До P12.2 (`c8d43a882`) | P12.2 (`8a87d9871`) | P12.3 (`fa705211a`) |
| --- | ---: | ---: | ---: |
| `gopacabra ops alloc` | 13 843 860 | 11 566 565 | 7 046 492 |
| `note_alloc_pinned` | 6 751 557 | 4 551 451 | 110 280 |
| `note_alloc_rc-local` | 3 326 882 | 3 284 655 | 3 244 074 |
| `gopacabra live pinned`, байти | 88 263 290 | 70 661 606 | 3 421 540 |
| `gopacabra live rc-local`, байти | 31 370 378 | 31 049 403 | 30 761 347 |
| RSS стала, КіБ | — | 172 032 | 101 120 |

Від початку фази виділень `pinned` на запит стало 11 замість 675, а всіх виділень — 705 замість 1 384. Тепер live визначає `rc-local`: 3 076 байтів на запит проти 342 у `pinned`. Різницю rps між прогонами (5 606 і 4 889) цей замір не оцінює: прогін один.

Залишок `pinned` за виділеннями:

| Місце | Виділень | Форма |
| --- | ---: | --- |
| `runtime/internal/runtime/z_string.go:123` (`StringToBytes`) | 59 849 | рантайм |
| `sqlite_g_0000000000000900.go:18175`, `:18181` (`_sqlite3VdbeExec`) | по 51 054 | `unsafe.Pointer(&iB)` у `_sqlite3AddInt64`, `&x1` у `_applyNumericAffinity`: ланцюжок викликаних виходить за правило |
| `sqlite_g_0000000000003d00.go:5308`, `:5309`; `sqlite_g_000000000003fd6b.go:9163` | ~2 480 кожне | — |

Форми `uintptr(unsafe.Pointer(&x))` (P12.4) серед головних місць залишку немає.

**ASan.** Збірка сервісу `-O0 -asan` на `fa705211a`, `ASAN_OPTIONS=detect_leaks=0`:
- `llgo test -O0 -asan -v .` — 32 PASS, 0 FAIL, без звітів;
- сценарій `-c 64 -n 100000` — `errors=0`, stderr порожній, код 0.

Не перевірено: darwin.

## P12.5 Залишок live (2026-11-24)

### Розклад до виправлень

Сервіс, збірка `-tags gopacabrastat,gopacabraheap` на `fa705211a` (P12.3), сценарій P11.6 (`-c 64`). Знімки купи після 10 000 і 30 000 запитів (`GOPACABRA_HEAP_OUT`), різниця — `heapdiff.py`.

Приріст live між знімками за категоріями, у байтах на запит:

| Категорія | Байтів на запит |
| --- | ---: |
| `rc-local` | ~2 770 |
| `pinned` | 328 |
| `rc-shared` | 179 |
| `cyclic` | 70 |

Головні місця приросту — функції, що виділили живий блок, у байтах на запит:

| Місце | Байтів на запит | Примітка |
| --- | ---: | --- |
| `Xmemcpy` (`modernc.org/libc`) | 705 | |
| `Xmemset` | 618 | |
| `netpoll` | 546 | рантайм |
| `UintptrMalloc` | 339 | |
| `reflect.unpackEface` | 292 | |
| `sqlite_g_0000000000000900.go:18181` | 284 | `pinned` |
| `Xstrlen` | 259 | |
| `UintptrFree` | 245 | |
| `database/sql.convertAssignRows` | 212 | |

Функції `libc` тут — місця, де ccgo-код тримає комірку змінної, яку захоплює defer-замикання трасування:

```go
if __ccgo_strace {
	defer func() { trc("-> %v", r) }()
}
```

`__ccgo_strace` — стала `false`, але `ssa` будує комірку `r` до перевірки умови. Компілятор не звільняв місце, якщо серед його користувачів був `*ssa.Defer`. Тому кожен виклик лишав 8 байтів `rc-local`.

### P12.5.1 Комірка, яку тримає defer-замикання

Правило — у `gopacabraFreeAfter` (`cl/gopacabra_free.go`), коміт `108ca546c`:
- місце, яке тримає defer, живе до `RunDefers`: кожен `RunDefers` і `Panic` функції стає його використанням;
- `free` стоїть після `RunDefers`;
- на шляху `panic` відкладені виклики біжать після `Panic`, тому там `free` немає і місце лишається живим.

Продовження після `RunDefers` у LLVM досяжне і з `setjmp`. Тому місце звільняється, лише якщо його алокація стоїть у блоці входу до першого defer (`deferEntrySite`). Перша версія без цієї умови не пройшла перевірку LLVM: «Instruction does not dominate all uses» у функціях рантайму з defer у циклі. Defer у чужий стек (range-over-func) правило не покриває.

Тест `TestGopacabraDeferClosureCell` (`252dff622`) з фікстурою `gopacabra/bytes/deferclosure` до виправлення червоний: `constdefer live rc-local delta 512 want 0`. ASan-фікстура `gopacabra/asan/deferclosure` охоплює defer завжди, під умовою, у циклі й на шляху `recover`.

Сервіс, `-tags gopacabrastat`, N = 10 000, один прогін:

| Лічильник | P12.3 (`fa705211a`) | P12.5.1 (`108ca546c`) |
| --- | ---: | ---: |
| `gopacabra live rc-local`, байти | 30 761 347 | 12 186 929 |
| `gopacabra live rc-shared`, байти | — | 2 135 950 |
| `gopacabra live cyclic`, байти | — | 987 976 |
| `gopacabra live pinned`, байти | 3 421 540 | 3 422 680 |
| `gopacabra ops alloc` | 7 046 492 | 7 167 994 |
| `note_alloc_rc-local` | 3 244 074 | 3 306 853 |
| RSS стала, КіБ | 101 120 | 75 008 |

Прогін — `errors=0`, 5 117 rps, код 0. Live `rc-local` упав на 60%, до ~1.2 КБ на запит.

**ASan.** Збірка сервісу `-O0 -asan` на `108ca546c`, `ASAN_OPTIONS=detect_leaks=0`:
- `llgo test -O0 -asan -v .` — 32 PASS, 0 FAIL, без звітів;
- сценарій `-c 64 -n 100000` — `errors=0`, stderr порожній, код 0.

### Залишок після P12.5.1

Оцінка за рядками `gopacabra site`: виділення місця без `dec`/`free` на тому ж місці. Це евристика за кількістю, не байти.

| Місце | Виділень без звільнення | Категорія і форма |
| --- | ---: | --- |
| `database/sql/convert.go:247` (`convertAssignRows`) | 134 572 | `rc-local closure`: параметр, який захоплює замикання `rows.cancel = func(){...}` (`:404`), що зберігається в полі |
| `sqlite_g_0000000000000900.go:18181`, `:18175` | по 51 054 | `pinned` (P12.3, ланцюжок викликаних) |
| `encoding/json/v2/arshal_default.go:231` | 48 539 | `rc-local call` |
| `modernc.org/libc/libc_all.go:88` (`GoString`) | 41 273 | `rc-local return` |
| `gopacabra/stand/rest/main.go:195`, `:196` | по 39 980 | `rc-local shared` |
| `database/sql/sql.go:3491` (`Rows.close`) | 21 118 з 42 236 | `rc-local closure` (`withLock`) |
| `sql.go:1742`, `:1776` | 10 559 | `cyclic closure` |
| `sql.go:1800`, `stmt.go:290` (sqlite) | 10 559 | `rc-shared` |
| `rows.go:65` (sqlite) | 10 559 | `rc-local closure` |
| `stmt.go:28`, `:32`, `conn.go:554`, `:1269` (sqlite), `fasthttp/header.go:3153` | ~11 000 кожне | — |

Задачі виправлень — P12.5.2 і далі в `PLAN.md`.

**Нестабільна фікстура.** `gopacabra/accountshard` падає приблизно в 10% прогонів: на 60 прогонах — 9 у збірці без P12.5.1 і 5 з ним. Різниця live `rc-local` між знімками — 136 байтів, кратна одному запуску потоку. Рядок `thread_unix.go:64` показує 8, 9 або 10 потоків на прогін. Найімовірніше, M:N запускає потік у вікні між знімками. P12.5.1 цієї нестабільності не спричинив.

Не перевірено: darwin; розклад у байтах після P12.5.1 (`heapdiff.py`).

### P12.5.2 Параметр, захоплений замиканням у полі

`database/sql.convertAssignRows` захоплює параметр `rows` замиканням `rows.cancel = func(){...}` лише в гілці курсора (`driver.Rows`). `ssa` будує комірку параметра в блоці входу на кожному виклику. Census вважав, що замикання, яке йде з функції (у поле, у виклик, у `return`, у горутину, канал чи глобальну змінну), забирає комірку на всіх шляхах. Тому `Dec` не ставився взагалі, навіть на шляхах без замикання.

Правило:
- `census` повертає такі замикання в `Placement.Escapes` разом з `DecUses` місця, а не прибирає `DecUses`;
- `gopacabraFreeAfter` (`cl/gopacabra_free.go`) не ставить `Dec` у точці, де замикання могло вже виконатись: у блоках, досяжних з його блоку, і в його блоці після нього;
- на решті шляхів комірка звільняється як звичайно.

Форму `return` правило не покриває: місця, досяжні з результату, `exitSites` блокує для `Dec` на всіх шляхах. Серед місць залишку сервісу цієї форми немає.

Тест `TestGopacabraClosureFieldCell` з фікстурою `gopacabra/bytes/closurefield` до виправлення червоний: `field live rc-local delta 512 want 0`. ASan-фікстура `gopacabra/asan/closurefield` викликає замикання з поля, з `return` і створене в циклі після повернення функції.

Сервіс, `-tags gopacabrastat`, N = 10 000, один прогін:

| Лічильник | P12.5.1 (`108ca546c`) | P12.5.2 |
| --- | ---: | ---: |
| `gopacabra live rc-local`, байти | 12 186 929 | 11 106 192 |
| `gopacabra live pinned`, байти | 3 422 680 | 3 428 568 |
| `convert.go:247`, `alloc_counted` | 134 572 | 134 572 |
| `convert.go:247`, `dec_accounted` | — | 134 572 |

Прогін — `errors=0`, код 0. Місце `convert.go:247` звільняється на кожному виклику, live `rc-local` упав на 1.08 МБ (8 байтів комірки на виклик).

**ASan.** Збірка сервісу `-O0 -asan`, `ASAN_OPTIONS=detect_leaks=0`:
- `llgo test -O0 -asan -v .` — 32 PASS, 0 FAIL, без звітів;
- сценарій `-c 64 -n 100000` — `errors=0`, stderr порожній, код 0.


### P12.5.6 Розклад у байтах після P12.5.2 і буфер подій `netpoll`

Сервіс, збірка `-tags gopacabrastat,gopacabraheap`, сценарій `-c 64 -n 10000`. Знімок купи — при виході (`GOPACABRA_HEAP_OUT`), розбір — `heapsym.py -n 10000`. Сюди ж входять одноразові виділення старту, наприклад таблиця рядків `symtab.go` (~400 Б на запит при N = 10 000).

Головні місця на P12.5.2 (`67304eea8`), live 6488 Б на запит:

| Місце | Байтів на запит | Категорія |
| --- | ---: | --- |
| `runtime.netpoll` (`netpoll_epoll_gopacabramn_llgo.go:81`), усі стеки | 2900 | `unplaced` |
| `reflect.unpackEface` (`value.go:182`) з `convert.go:456` і `:465` | 282 | `unplaced` |
| `sqlite_g_…900.go:18181` | 224 | — |
| `encoding/json.Marshal` → `bytes.Clone` | 209 | — |
| `main.go:196` сервісу | 196 | — |
| `modernc.org/sqlite.newRows` (`rows.go:65`) | 177 | `rc-local closure` |
| `database/sql` `sql.go:1800` | 165 | `rc-shared` |

**`unplaced`.** Census розставляє лише функції оригінального пакета: `prepareGopacabraFree` бере `ssaPackageFuncs(pkg)`, а функції оверлея (`patch.Alt`: `runtime/internal/lib/runtime`, `.../reflect` тощо) у розстановку не потрапляють. Змінна, чия адреса там тікає, виділяється в купі без звільнення. Лічильники `gopacabra live` такі місця не бачать: вони не мають категорії. Видно їх лише в рядках `gopacabra site … unplaced` і в профілі купи.

**P12.5.6.1.** `netpoll` оголошував `var events [128]epollEvent` (1536 Б) і передавав `&events[0]` у `C.epoll_wait`, тож кожен виклик з планувальника M:N (`mnFindRunnable`, `mnIdle`) лишав блок у купі. Те саме — з `var ev` у `netpollOpen` і `netpollClose` та з `events`/`ts` у версії kqueue. Ці буфери тепер виділяються на C-стеку через `c.Alloca`, як `nanotime_linux_llgo.go`.

Тест `TestGopacabraNetpollEvents` збирає фікстуру `gopacabra/bytes/netpoll` (1000 обмінів через loopback) з `gopacabrastat` і читає рядки `gopacabra site` файлів `netpoll_*`. До виправлення: `netpoll_epoll_gopacabramn_llgo.go:81 alloc: 4051`.

Після виправлення, той самий сценарій:

| Лічильник | P12.5.2 | P12.5.6.1 |
| --- | ---: | ---: |
| live за профілем, Б на запит | 6488 | 3524 |
| місць `netpoll` у профілі | 6 | 0 |

Головні місця після P12.5.6.1:

| Місце | Байтів на запит | Задача |
| --- | ---: | --- |
| `reflect.unpackEface` (`value.go:182`), 190 085 виділень | 284 | P12.5.6.2 |
| `sqlite_g_…900.go:18181` | 273 | — |
| `sql.go:1800` | 199 | P12.5.5 |
| `main.go:195`/`:196` сервісу | 326 | — |
| `encoding/json.Marshal` → `bytes.Clone` | 183 | P12.5.4 |
| `newRows` (`rows.go:65`) | 135 | P12.5.5 |

**P12.5.6.2.** `prepareGopacabraFree` тепер розставляє функції пакета разом із функціями його оверлея (`patch.Alt`): вони компілюються в тому самому контексті. Оверлей самого `runtime` census свідомо не розставляє: пули й звільнення рантайму потрапили б у `Live` і `Freed` поряд із місцями програми. Його місця лишаються `unplaced`; найбільше з них — таблиця рядків `symtab.go`, одноразове виділення старту.

Тест `TestGopacabraOverlaySites` збирає фікстуру `gopacabra/bytes/overlaysite` (4000 викликів `reflect.ValueOf`) з `gopacabrastat` і шукає рядки `gopacabra site … unplaced` файлів `runtime/internal/lib/` (крім `runtime/internal/lib/runtime/`) з лічильником понад 100.

Після виправлення (`c71de8c90`), той самий сценарій, один прогін:

| Лічильник | P12.5.6.1 | P12.5.6.2 |
| --- | ---: | ---: |
| live за профілем, Б на запит | 3524 | 3247 |
| `reflect.unpackEface` (`value.go:182`) у профілі, Б на запит | 284 | 0 |
| рядків `unplaced` поза оверлеєм `runtime` з лічильником понад 100 | — | 2 (`fiber/v3` `helpers.go:995` і `:1078`, по ~4000 виділень) |

Два місця `fiber` — не оверлей; чому census їх не розставив, не з'ясовано.

**P12.5.5.1.** Місця `sql.go:1742` і `:1776` — комірки параметрів `db *DB` і `dc *driverConn`, які захоплюють замикання для `db.retry` і `withLock`. Census позначав їх `cyclic: closure`, бо `DB` через `driverConn.db` досягає себе; такі місця лічильника не мають, а `DecCyclic` їх не бере (полів обходу в комірки немає). Комірка змінної-вказівника — одне слово, а не вузол типу: в цикл вона входить, лише коли з `*T` досяжний вказівник на саму комірку, `**T`. Тепер її позначка — `rc-local: closure`, і вона звільняється за правилом P12.5.2.

Сервіс після виправлення (`5118c22e9`), `gopacabrastat`, N = 10 000, один прогін:

| Лічильник | P12.5.6.2 | P12.5.5.1 |
| --- | ---: | ---: |
| `sql.go:1742`, `:1776`: `dec_accounted` на виділення | 0 | 1 |
| live `cyclic`, байтів | 993 544 | 784 520 |
| live `rc-local`, байтів | 11 106 743 | 11 076 831 |

Лічильник виділень цих місць зріс з 10 559 до 42 236: рядок тепер рахує всі комірки-вказівники функції, а не лише ту, що була `cyclic`. Байти в профілі купи не знімались.

Решту P12.5.5 поділено за причинами: P12.5.5.2 — `Rows` (`sql.go:1800`, `:1769`, `:3084`), що живе до `Rows.Close` у коді користувача; P12.5.5.3 — `rows` драйвера sqlite (`rows.go:65`, `stmt.go:290`).

**P12.5.3 зводиться до P12.5.5.** Рядки `GoString` (`libc_all.go:88`, ~20 Б на запит) живуть у зрізах `columns` і `decltypes` об'єкта `rows`, створеного в `newRows`. Сам `rows` лишається живим (P12.5.5), тож окремого виправлення рядки не потребують.

**ASan.** Сервіс `-O0 -asan`, `ASAN_OPTIONS=detect_leaks=0`, сценарій `-c 64 -n 100000` — `errors=0`, stderr порожній, код 0.
