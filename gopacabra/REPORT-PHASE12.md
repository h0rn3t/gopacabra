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
