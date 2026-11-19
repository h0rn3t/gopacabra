# rest — сервіс фази 11 (P11.4)

Звичайний REST-сервіс на fiber v3 і SQLite через `modernc.org/sqlite` (чистий Go, збирається з `CGO_ENABLED=0`). Окремий модуль: його залежності не потрапляють у `go.mod` llgo, а `go test ./...` з кореня його не бачить. Вибір і умови стенду — `INTENT.md`, «Сервіс фази 11».

| Маршрут | Що робить |
| --- | --- |
| `GET /healthz` | 200 `ok` |
| `POST /items` `{"name","qty"}` | 201 з `id`; 400 на невалідний JSON, порожній `name`, від'ємний `qty` |
| `GET /items/:id` | 200, 404, 400 на нечисловий `id` |
| `PUT /items/:id` `{"name","qty"}` | 200 з новими значеннями, 404, 400 |
| `DELETE /items/:id` | 204, 404, 400 |
| `GET /items?after=&limit=` | сторінка за `id` (keyset), `limit` 1–1000, типово 100; порожня — `[]` |

Записи йдуть через пул з одним з'єднанням, читання — через окремий пул (WAL): з кількома з'єднаннями запису SQLite під навантаженням віддає `SQLITE_BUSY`.

## Запуск

З кореня репозиторію:

```sh
cd gopacabra/stand/rest
go test -race .                                    # контракт API під gc
CGO_ENABLED=0 go build -o $BIN/rest-gc .
(cd ../../.. && go build -o $BIN/service ./gopacabra/bench/service)
$BIN/rest-gc -addr 127.0.0.1:18090 -db /tmp/rest.sqlite &   # база має бути порожньою
$BIN/service -url http://127.0.0.1:18090 -c 64 -n 100000    # сценарій; код 0 — без неочікуваних відповідей
```

Сценарій (`gopacabra/bench/service`): засів 1000 записів, далі 60% `GET /items/{id}`, 20% `GET /items?after=&limit=20`, 10% `POST`, 5% `PUT`, 5% `DELETE` — кожна відповідь звіряється з очікуваною.
