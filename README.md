<p align="center"><img src="doc/gopacabra-logo.png" alt="Логотип Gopacabra" width="320"></p>

Gopacabra — компілятор Go без трасувального GC
=====

> Компілятор Go 1.27 без трасувального збирача сміття: незмінений Go-код, керування пам'яттю як у Rust.
> Основа — форк [llgo](https://github.com/goplus/llgo). Опис llgo від upstream — у [LLGO.md](LLGO.md).

Попередня назва проєкту — OwnGo. Gopacabra — незалежний проєкт, не пов'язаний з проєктом Go чи Google і не схвалений ними; Go — торговельна марка Google LLC.

## Задумка

Звичайна Go-програма (модулі, залежності, стандартна бібліотека) збирається без змін у нативний бінарник, де пам'ять звільняє не GC у фоні, а код, який вставив компілятор. Більшість об'єктів звільняється детерміновано — у точці останнього використання. Програма поводиться так само, як зібрана `gc`: специфікація Go не вимагає GC, змінюється лише час спрацювання `runtime.AddCleanup` і фіналізаторів.

Навіщо: передбачувані p99 і RSS без пауз і без запасу пам'яті під збирач — для сервісів, CLI і батч-програм, які вже написані на Go.

## Модель пам'яті: каскад

Для кожної алокації в купі прохід володіння обирає найсильнішу стратегію, яку може довести:

| Рівень | Коли | Механізм |
| --- | --- | --- |
| 1. Стек | об'єкт не втікає | escape analysis `go/ssa` + власний прохід |
| 2. Статичне володіння | доведено єдиного власника і точку останнього використання | компілятор вставляє `free` |
| 3. Підрахунок посилань | усе інше | inc/dec з виведенням позичань (Perceus); неатомарний, якщо об'єкт не покидає горутину |
| 4. Збирач циклів | лише типи, циклічні за графом типів | trial deletion (як ORC у Nim) |
| Закріплення | `unsafe.Pointer` → `uintptr`, непрозорий `reflect` | не звільняється; попередження компілятора |

## Принципи

1. **Незмінений код.** Що збирає `gc` 1.27, те збирає й Gopacabra — без анотацій і нового синтаксису.
2. **Коректність понад економію.** Не доведено володіння — слабша стратегія (RC, закріплення). Передчасний `free` неприпустимий.
3. **Кожен `free` перевіряється.** Debug-збірки — під ASan, тести — диференційні проти `gc`.
4. **Мінімальний диф з upstream llgo.** Новий код — у [`gopacabra/`](gopacabra), зміни рантайму — під тегом збірки `gopacabra`.
5. **Спершу виміри.** Кожна фаза починається з метрик і закінчується гейтом.

Не-цілі: гарантії borrow checker, власні фронтенд чи рантайм з нуля, інтероп із C (cgo за замовчуванням вимкнено) і Python (видалено).

## Архітектура

```
Go 1.27 ─▶ фронтенд llgo (go/packages + go/types + go/ssa)
        ─▶ прохід володіння: стратегія для кожної heap-алокації, точки free / inc / dec
        ─▶ codegen llgo (LLVM 23): виклики runtime.Free і RC
        ─▶ рантайм (тег gopacabra): алокатор + Free + RC + збирач циклів
```

Горутини — планувальник M:N з логічними процесорами, як у `gc`: типовий для виконуваних файлів на linux/amd64, linux/arm64 і darwin. Стеки мають фіксований резерв у віртуальній пам'яті й не переміщуються. Бібліотечні режими (`c-shared`, `c-archive`) і тег `gopacabra1to1` лишають модель llgo: потік ОС на горутину.

## Стан

Готові фази 0–10: виміри на профілях сервісів, `free` для статичного володіння, неатомарний RC для локальних об'єктів, звільнення неявних алокацій (`append`, конкатенація рядків), збирач циклів у `DecCyclic`; на корпусі програм Benchmarks Game — алокації без місця в census, внутрішні вказівники, атомарний RC для об'єктів, спільних між горутинами, облік закріпленого `unsafe.Pointer` з попередженням компілятора.

Зараз — фаза 11, паритет з `gc`. Готово:

- **Витік закріпленого (P11.0, P11.0.1).** Рантайм хешує ключі мап без купової копії, а змінна, чию адресу лише перетворюють в інший тип (`*(*uint64)(unsafe.Pointer(&f))`, як у `math.Float64bits` і `strconv`), лежить на стеку, як у `gc`.
- **`GOROOT/test` під тегом `gopacabra` (P11.2, P11.3).** Режим `ci` (`.`, `fixedbugs`, 830 випадків) проти llgo без тегу: з `gopacabra` лишились 3 регресії, і всі не застосовні — `init1.go` чекає зростання `runtime.NumGC`, `mallocfin.go` і `tinyfin.go` — фіналізаторів, а трасувального збирача немає за задумом. Під `-O0 -asan` — жодного `heap-use-after-free`, `double-free` чи `attempting free`; ще 4 падіння — обмеження ASan, `-O0` і ліміту пам'яті раннера. Деталі — [gopacabra/REPORT-PHASE11.md](gopacabra/REPORT-PHASE11.md).

- **Сервіс фази 11 (P11.4).** REST-сервіс [`gopacabra/stand/rest`](gopacabra/stand/rest) на fiber v3 і `modernc.org/sqlite`.
- **`gopacabra` — єдина збірка (P11.5.0, триває).** Тег `gopacabra` отримує кожна збірка; BDWGC (P11.5.0.1) і `tinygogc` (P11.5.0.2) видалено: wasm теж збирається з рантаймом `gopacabra`. Baremetal-цілі (ESP — P11.5.0.5, решту плат і MCU — P11.5.0.6) видалено: лишились нативні ОС і WebAssembly. Дефекти рантайму, знайдені тестами llgo, закрито (P11.5.0.3).

Далі — перша збірка сервісу (P11.5: ланцюжок `modernc.org/sqlite` упирається в переклад асемблера і `__ccgo_fp`), його тести під `gopacabra` і замір p99, RSS і пропускної здатності проти `gc` (P11.6–P11.8).

Окремий трек — швидкодія до рівня Rust/C на незміненому Go-коді, етапи в [gopacabra/ROADMAP-PERF.md](gopacabra/ROADMAP-PERF.md). Етапи A–D і F закрито, гейт E чекає рішення, етап G (M:N) злито в типову збірку й доопрацьовується. Серед результатів:

- облік `gopacabra` і лічильники профілю пам'яті розкладено по шардах потоків (S2, S3): CPU-час Go #2 binary-trees `18` на 8 ядрах — 40.9 → 4.2 с;
- профіль треку — `-O2 -lto thin` (P-NEW-01): fannkuch-redux `12` — 1.15× від Rust #6; типовий профіль лишається `-Os`;
- лічильники операцій рантайму під тегом `gopacabrastat` (P-NEW-02.0) — база етапу C (зведення володіння, RC elision, стек);
- рантайм регіонів `RegionCreate`/`RegionAlloc`/`FreeRegion` (P-NEW-05.3) — перший крок етапу D; codegen, що кладе дерева в регіон, — наступні задачі.

Бенчмарки (Debian 13 amd64, Xeon Gold 6240R, 8 ядер, Go 1.27.1, LLVM 23.1.5, rustc 1.85.1; медіана з 10 прогонів із чергуванням варіантів). Усі варіанти заміряно разом 2026-11-07 на ревізії `2a8b4583f`, після гейта F; RSS `gopacabra` — профілю `-O2 -lto thin`:

| Програма | `go` | `gopacabra` `-Os` | `gopacabra` `-O2 -lto thin` | Rust | C | Пікова RSS: `go` / `gopacabra` / Rust / C |
| --- | --- | --- | --- | --- | --- | --- |
| [binary-trees 21](gopacabra/REPORT-BENCH.md#binary-trees-21), Go #2 без змін | 5.36 с | 1.33 с | 1.22 с | — | 5.33 с (#5: `malloc` + pthreads) | 631 / 306 / — / 343 МБ |
| binary-trees 21 на аренах ([`binarytreesarena`](gopacabra/bench/binarytreesarena)) | 0.42 с | 0.49 с | 0.43 с | 0.68 с (#5: bumpalo + rayon) | 1.04 с (#2: пули APR + OpenMP) | 227 / 359 / 322 / 175 МБ |
| [fannkuch-redux 12](gopacabra/REPORT-BENCH.md#fannkuch-redux-12) | 3.22 с | 6.13 с | 2.41 с | 2.21 с (#6: rayon, SSE4) | 1.14 с (#6: pthreads, SSE4.1) | 2 / 5 / 2 / 2 МБ |
| [mandelbrot 16000](gopacabra/REPORT-BENCH.md#mandelbrot-16000) | 1.50 с | 0.89 с | 0.63 с | 0.54 с (#4: rayon) | 0.61 с (#6: OpenMP, SSE2) | 33 / 36 / 33 / 33 МБ |

Вивід усіх програм побайтно збігається з очікуваним. Таблиці binary-trees — окремо за стратегією пам'яті: Go #2 і C #5 звільняють вузли по одному, а арена Go, Rust #5 і пули C тримають дерево цілим; висновку між рядками немає. Go #2 під `gopacabra` до S2 і S3 ішов 49.2 с: час з'їдали атомарні лічильники обліку і профілю на спільних кеш-лініях. Після S2 і S3 — 8.03 с; регіони етапу D (дерева робітників — у регіоні) і відкладене повернення сторінок алокатора етапу F дали 1.22 с, у 4.4 раза швидше за `go` і C #5 ([gopacabra/REPORT-STAGEF.md](gopacabra/REPORT-STAGEF.md)). `fannkuch-redux` — порт Rust #6 на Go, `mandelbrot` — порт Rust #4; програми Rust і C — з Benchmarks Game без змін коду. Повні таблиці (усі профілі, llgo без тега, CPU user/sys, розкид, розмір бінарника, час збірки) і JSON — у [gopacabra/REPORT-BENCH.md](gopacabra/REPORT-BENCH.md), [gopacabra/bench/profiles](gopacabra/bench/profiles), [gopacabra/bench/baseline](gopacabra/bench/baseline) і [gopacabra/bench/stagef](gopacabra/bench/stagef) (замір цієї таблиці). Команда: `gopacabra/bench/compare.py binarytrees|fannkuchredux|mandelbrot [--profiles]`.

## Збірка і запуск

Потрібні LLVM 23 і LLD 23 першими в `PATH` (форк використовує LLVM 23, upstream — 22), `LLGO_ROOT` — корінь репозиторію:

```bash
export LLGO_ROOT=$PWD
go install ./cmd/llgo
llgo build -o app ./path/to/pkg
llgo build -O2 -lto thin -o app ./path/to/pkg   # профіль треку швидкодії
llgo run -O0 -asan .   # debug-збірка під ASan
```

Тег `gopacabra` додається до кожної збірки сам, вимкнути його не можна. На amd64 типовий `GOAMD64` — `v3` (AVX2); явне значення `GOAMD64` має пріоритет. Звичайна збірка нічого зайвого не друкує. Тег `gopacabrareport` додає на виході рядки `gopacabra freed` і `gopacabra live` та попередження `gopacabra: pinned` під час збірки; `gopacabrastat` (вмикає й `gopacabrareport`) — лічильники `gopacabra ops`, `alloc` і `site`. Під ASan потрібен `ASAN_OPTIONS=detect_leaks=0`: без GC LeakSanitizer вважає витоком кожен живий об'єкт на виході.

Окрім нативних ОС, лишились лише цілі WebAssembly: `-target wasm`, `wasi`, `wasip1`, `emscripten`, `emscripten-memory64`. Для крос-компіляції під інші ОС llgo сам завантажує ESP clang 23.

## Перевірки

- `go test ./...` — відомі падіння в [gopacabra/BASELINE.md](gopacabra/BASELINE.md); зміна не має додавати нових.
- `gopacabra/fixtures.sh [llgo]` — усі програми прийняття `gopacabra/*`: під `gopacabra/asan` фікстура `*uaf` має впасти зі звітом ASan, решта — код 0.
- CI форку перевіряє лише gopacabra: workflow `Gopacabra` (`go test` пакетів gopacabra, фікстури, `GOROOT/test` під `-tags=gopacabra` з додатковими xfail з [gopacabra/goroot-xfail.yaml](gopacabra/goroot-xfail.yaml)) і `Gopacabra binary-trees` (корпус бенчмарків). Upstream-перевірки llgo (wasm, релізи) у форку вимкнено.

## Документи

- [INTENT.md](INTENT.md) — мета, принципи, модель пам'яті, рішення гейтів фаз.
- [PLAN.md](PLAN.md) — задачі поточної фази з критеріями приймання.
- [PROGRESS.md](PROGRESS.md) — журнал сесій.
- [LLGO.md](LLGO.md) — опис llgo від upstream: підтримка Go, інтероп із C, залежності, встановлення.
- [gopacabra/BASELINE.md](gopacabra/BASELINE.md) — відомі падіння `go test ./...`.
- [gopacabra/REPORT-BENCH.md](gopacabra/REPORT-BENCH.md) — корпус бенчмарків і профілі збірки.
- [gopacabra/ROADMAP-PERF.md](gopacabra/ROADMAP-PERF.md) — етапи треку швидкодії.
- `gopacabra/REPORT-*.md` — заміри фаз і бенчмарків.
