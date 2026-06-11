#!/usr/bin/env bash
set -euo pipefail

RUNS="${RUNS:-5}"

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

case "$RUNS" in
  ''|*[!0-9]*) fail "RUNS must be a positive integer" ;;
esac
[ "$RUNS" -gt 0 ] || fail "RUNS must be greater than zero"

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

"$script_dir/reset-sample-api.sh"

latencies=""
passes=0
failures=0

printf '\n## Beacon Benchmark Series\n\n'
printf 'Runs: %s\n\n' "$RUNS"

run=1
while [ "$run" -le "$RUNS" ]; do
  printf '### Run %s/%s\n' "$run" "$RUNS"
  output="$("$script_dir/benchmark-scale-latency.sh")" || {
    printf '%s\n' "$output"
    failures=$((failures + 1))
    run=$((run + 1))
    continue
  }
  printf '%s\n' "$output"
  latency="$(printf '%s\n' "$output" | awk -F'[: ]+' '/BeaconPolicy latency:/ { print $3; exit }' | sed 's/ms$//')"
  result="$(printf '%s\n' "$output" | awk -F': ' '/Result:/ { print $2; exit }')"
  if [ -n "$latency" ]; then
    latencies="${latencies}${latencies:+ }${latency}"
  fi
  case "$result" in
    PASS|PASS_MAX_REACHED) passes=$((passes + 1)) ;;
    *) failures=$((failures + 1)) ;;
  esac
  printf '\n'
  run=$((run + 1))
done

if [ -n "$latencies" ]; then
  stats="$(printf '%s\n' "$latencies" | awk '
    {
      for (i = 1; i <= NF; i++) {
        value = $i + 0
        if (count == 0 || value < min) min = value
        if (count == 0 || value > max) max = value
        sum += value
        count++
      }
    }
    END {
      if (count > 0) printf "%d %d %.0f", min, max, sum / count
    }
  ')"
  min_latency="$(printf '%s' "$stats" | awk '{print $1}')"
  max_latency="$(printf '%s' "$stats" | awk '{print $2}')"
  avg_latency="$(printf '%s' "$stats" | awk '{print $3}')"
else
  min_latency="n/a"
  max_latency="n/a"
  avg_latency="n/a"
fi

printf '## Series Summary\n\n'
printf 'Passes: %s\n' "$passes"
printf 'Failures: %s\n' "$failures"
printf 'Min latency: %sms\n' "$min_latency"
printf 'Max latency: %sms\n' "$max_latency"
printf 'Average latency: %sms\n' "$avg_latency"

[ "$failures" -eq 0 ] || exit 1
