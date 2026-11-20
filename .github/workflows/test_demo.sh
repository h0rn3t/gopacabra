#!/bin/bash
set -e

# Keep the scheduler deliberately small: directory consolidation owns coverage,
# while this script only selects the few specialized profiles and runs them.
jobs="${LLGO_DEMO_JOBS:-4}"
case "$jobs" in
  ""|*[!0-9]*|0)
    echo "LLGO_DEMO_JOBS must be a positive integer" >&2
    exit 2
    ;;
esac

llgo_run_flags=()
if [ -n "${LLGO_DEMO_LLGORUN_FLAGS:-}" ]; then
  read -r -a llgo_run_flags <<< "${LLGO_DEMO_LLGORUN_FLAGS}"
fi

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT

run_dirs=()
cases=()
use_lto=0
use_globaldce=0
use_deadcodedrop=0
for flag in "${llgo_run_flags[@]}"; do
  case "$flag" in
    -lto=full) use_lto=1 ;;
    -globaldce) use_globaldce=1 ;;
    -deadcodedrop) use_deadcodedrop=1 ;;
  esac
done

if [ "$use_lto" -eq 1 ] && [ "$use_globaldce" -eq 1 ]; then
  cases=(
    ./_demo/c/asmfullcall
    ./_demo/c/cgofull
    ./_demo/c/cabisret
    ./_demo/c/cppintf
    ./_demo/go/cabi
    ./_demo/go/export
    ./_demo/go/reflect
    ./_demo/go/stdlib
    ./_demo/go/sync
  )
elif [ "$use_deadcodedrop" -eq 1 ]; then
  cases=(
    ./_demo/c/asmfullcall
    ./_demo/c/cgofull
    ./_demo/c/cabisret
    ./_demo/go/cabi
    ./_demo/go/reflect
    ./_demo/go/stdlib
    ./_demo/go/sync
  )
else
  search_dirs=(./_demo/go/* ./_demo/c/*)
  for d in "${search_dirs[@]}"; do
    if [ -d "$d" ] && [ -n "$(ls "$d"/*.go 2>/dev/null)" ]; then
      cases+=("$d")
    fi
  done
fi

for d in "${cases[@]}"; do
  if [ "$d" = "./_demo/c/llama2-c" ] && [ "${LLGO_RUN_MODEL_DEMOS:-0}" != "1" ]; then
    echo "SKIP $d (model demo runs in scheduled Model Demo workflow)"
    continue
  fi
  if [ "$d" = "./_demo/c/helloc" ]; then
    echo "SKIP $d (WASI C-FFI demo runs in the cross-compile workflow)"
    continue
  fi
  run_dirs+=("$d")
done

total="${#run_dirs[@]}"
failed=0
failed_cases=""

run_case() {
  local dir="$1"
  local output_dir="$2"
  local output executable
  local -a cmd
  echo "Testing $dir"

  mkdir -p "$output_dir"
  output="$output_dir/demo"
  cmd=(llgo build)
  cmd+=("${llgo_run_flags[@]}")
  cmd+=("-p=1" "-o" "$output" ".")
  if ! (cd "$dir" && GOTOOLCHAIN=local GOWORK=off "${cmd[@]}"); then
    echo "FAIL"
    return 1
  fi
  executable="$output"
  if (cd "$dir" && "$executable"); then
    echo "PASS"
    return
  fi
  echo "FAIL"
  return 1
}

if [ "$jobs" -le 1 ] || [ "$total" -le 1 ]; then
  idx=0
  for i in "${!run_dirs[@]}"; do
    d="${run_dirs[$i]}"
    idx=$((idx+1))
    output_dir="$tmp_root/$(printf '%04d' "$idx")"
    if ! run_case "$d" "$output_dir"; then
      failed=$((failed+1))
      failed_cases="$failed_cases\n* :x: $d"
    fi
  done
else
  active_pids=()
  active_dirs=()
  active_logs=()
  idx=0

  wait_for_one() {
    finished_pid=""
    if [ "${BASH_VERSINFO[0]}" -gt 5 ] || { [ "${BASH_VERSINFO[0]}" -eq 5 ] && [ "${BASH_VERSINFO[1]}" -ge 1 ]; }; then
      if wait -n -p finished_pid; then
        finished_status=0
      else
        finished_status=$?
      fi
    fi
    if [ -z "${finished_pid:-}" ]; then
      # wait -n can omit an already completed job. Reap a
      # known PID to preserve its exit status and always drain the queue.
      # This also supports Bash 3.2, which has no wait -n.
      finished_pid="${active_pids[0]}"
      if wait "$finished_pid"; then
        finished_status=0
      else
        finished_status=$?
      fi
    fi
    for active_index in "${!active_pids[@]}"; do
      if [ "${active_pids[$active_index]}" = "$finished_pid" ]; then
        cat "${active_logs[$active_index]}"
        echo "Finished ${active_dirs[$active_index]} (exit $finished_status)"
        if [ "$finished_status" -ne 0 ]; then
          failed=$((failed+1))
          failed_cases="$failed_cases\n* :x: ${active_dirs[$active_index]}"
        fi
        unset 'active_pids[active_index]' 'active_dirs[active_index]' 'active_logs[active_index]'
        active_pids=("${active_pids[@]}")
        active_dirs=("${active_dirs[@]}")
        active_logs=("${active_logs[@]}")
        break
      fi
    done
  }

  for i in "${!run_dirs[@]}"; do
    d="${run_dirs[$i]}"
    idx=$((idx+1))
    log="$tmp_root/$(printf '%04d' "$idx").log"
    output_dir="$tmp_root/$(printf '%04d' "$idx")"
    echo "Starting $d"
    (run_case "$d" "$output_dir") >"$log" 2>&1 &
    pid=$!
    active_pids+=("$pid")
    active_dirs+=("$d")
    active_logs+=("$log")

    while [ "${#active_pids[@]}" -ge "$jobs" ]; do
      wait_for_one
    done
  done

  while [ "${#active_pids[@]}" -gt 0 ]; do
    wait_for_one
  done
fi

echo "=== Done"
echo "$((total-failed))/$total tests passed"

if [ "$failed" -ne 0 ]; then
  echo ":bangbang: Failed demo cases:" | tee -a result.md
  echo -e "$failed_cases" | tee -a result.md
  exit 1
else
  echo ":white_check_mark: All demo tests passed" | tee -a result.md
fi
