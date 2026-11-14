#!/usr/bin/env bash
# measure.sh VARIANT PORT: сценарій гейта G на одному бінарнику стенду.
# Бінарники fiberdb-VARIANT і load шукаються в BIN (типово — поруч зі скриптом).
# Кожна фаза — на свіжому процесі: падіння fanout не забирає навантаження,
# а VmHWM fanout не містить пам'яті навантаження.
# FANOUT — список n для /fanout (типово 1000 20000 100000); після кожного n —
# час, потоки і VmHWM. LOADS — клієнти послідовних навантажень по 5 с
# (типово 100 1000 1000); після кожного — VmRSS.
set -u
dir=${BIN:-$(dirname "$0")}; v=$1; port=$2
url=http://127.0.0.1:$port

start() {
	db=$(mktemp -d)/items.db
	"$dir/fiberdb-$v" -addr 127.0.0.1:$port -db "$db" 2>"$dir/$v.stderr" &
	pid=$!
	for _ in $(seq 50); do curl -s -o /dev/null $url/items && break; sleep 0.1; done
	curl -s -o /dev/null -X POST -H 'Content-Type: application/json' -d '{"name":"a","qty":1}' $url/items
}

stop() {
	kill -INT $pid 2>/dev/null; wait $pid; echo "$v exit=$?"; tail -3 "$dir/$v.stderr"
	rm -rf "$(dirname "$db")"
}

kib() { awk -v k="$1:" '$1 == k {print $2}' /proc/$pid/status 2>/dev/null; }

start
for n in ${FANOUT:-1000 20000 100000}; do
	t0=$(date +%s.%N)
	out=$(curl -s -m 120 -w ' http=%{http_code}' "$url/fanout?n=$n")
	t1=$(date +%s.%N)
	alive=$(kill -0 $pid 2>/dev/null && echo alive || echo dead)
	echo "$v fanout n=$n: $out time=$(awk "BEGIN{printf \"%.2f\", $t1-$t0}")s threads=$(ls /proc/$pid/task 2>/dev/null | wc -l) hwm_kib=$(kib VmHWM) server=$alive"
	[ $alive = dead ] && break
done
stop

start
for c in ${LOADS:-100 1000 1000}; do
	echo "$v load c=$c: $("$dir/load" -url $url/items/1 -c $c -d 5s 2>&1) rss_kib=$(kib VmRSS)"
done
stop
