#!/usr/bin/env bash
# Запускає кожну програму прийняття gopacabra/* і звіряє результат з очікуваним.
#
# Використання: gopacabra/fixtures.sh [LLGO]   (типово — llgo з PATH)
# GOPACABRA_TAGS — додаткові теги кожної збірки; GOPACABRA_TAGS=gopacabra1to1 — модель 1:1
# замість типової M:N.
#
# Очікування задає розташування фікстури:
#   gopacabra/asan/*uaf — під -O0 -asan процес має впасти зі звітом AddressSanitizer;
#   gopacabra/asan/*    — під -O0 -asan код 0 і жодного звіту;
#   решта           — код 0.
# Теги gopacabrastat, gopacabramn і gopacabra1to1 додаються, якщо їх вимагає рядок
# //go:build фікстури.
# Каталоги bench, census/cmd, third_party, stand (окремі модулі) і пакети site —
# не програми прийняття.
set -uo pipefail

llgo=${1:-llgo}
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

# Без GC LeakSanitizer бачить витоком кожен живий об'єкт на виході.
export ASAN_OPTIONS=${ASAN_OPTIONS:-detect_leaks=0}

# Відомі падіння, записані в PROGRESS.md. Фікстура зі списку, що пройшла, —
# теж помилка: її треба прибрати звідси.
declare -A known=(
	[gopacabra/counted]="links C.malloc_size, a macOS API absent on Linux"
	[gopacabra/runtimefree]="links C.malloc_size, a macOS API absent on Linux"
)

failed=()
count=0
while IFS= read -r dir; do
	count=$((count + 1))
	tags=gopacabra${GOPACABRA_TAGS:+,$GOPACABRA_TAGS}
	grep -q '^//go:build.*gopacabrastat' "$dir"/*.go && tags=$tags,gopacabrastat
	grep -q '^//go:build.*gopacabramn' "$dir"/*.go && tags=$tags,gopacabramn
	grep -q '^//go:build.*gopacabra1to1' "$dir"/*.go && tags=$tags,gopacabra1to1
	flags=(-tags "$tags")
	case $dir in gopacabra/asan/*) flags+=(-O0 -asan) ;; esac

	out=$("$llgo" run "${flags[@]}" "./$dir" 2>&1)
	code=$?
	report=no
	grep -q AddressSanitizer <<<"$out" && report=yes

	case $dir in
	gopacabra/asan/*uaf) [[ $code -ne 0 && $report == yes ]] ;;
	*) [[ $code -eq 0 && $report == no ]] ;;
	esac
	passed=$?
	if [[ -n ${known[$dir]:-} ]]; then
		if [[ $passed -ne 0 ]]; then
			echo "known $dir: ${known[$dir]}"
			continue
		fi
		echo "FAIL $dir passes but is listed as a known failure in gopacabra/fixtures.sh"
		failed+=("$dir")
		continue
	fi
	if [[ $passed -eq 0 ]]; then
		echo "ok   $dir"
		continue
	fi
	echo "FAIL $dir (exit $code, ASan report: $report)"
	tail -n 30 <<<"$out" | sed 's/^/    /'
	failed+=("$dir")
done < <(find gopacabra -name main.go \
	-not -path 'gopacabra/bench/*' -not -path 'gopacabra/census/cmd/*' \
	-not -path 'gopacabra/third_party/*' -not -path 'gopacabra/stand/*' -not -path '*/site/*' |
	xargs -n1 dirname | sort)

echo "${#failed[@]} of $count fixtures failed"
[[ ${#failed[@]} -eq 0 ]]
