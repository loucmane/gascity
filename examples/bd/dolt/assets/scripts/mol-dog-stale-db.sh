#!/usr/bin/env bash
# Deterministic stale-Dolt cleanup for the scheduled exec order. This is the
# same scan/threshold/apply decision formerly delegated to a dog agent, but it
# creates no work item and starts no model process.
set -euo pipefail
umask 077

GC="${GC_BIN:-gc}"
CITY="${GC_CITY_PATH:-${GC_CITY:-.}}"
RUNTIME_DIR="${GC_CITY_RUNTIME_DIR:-$CITY/.gc/runtime}"
REPORT_DIR="$RUNTIME_DIR/maintenance/mol-dog-stale-db"
MAX_ORPHANS="${GC_STALE_DB_MAX_ORPHANS:-20}"
WARN_THRESHOLD="${GC_STALE_DB_WARN_THRESHOLD:-5}"

case "$MAX_ORPHANS" in ''|*[!0-9]*) echo "stale-db: max threshold must be a non-negative integer" >&2; exit 2 ;; esac
case "$WARN_THRESHOLD" in ''|*[!0-9]*) echo "stale-db: warn threshold must be a non-negative integer" >&2; exit 2 ;; esac
command -v jq >/dev/null 2>&1 || { echo "stale-db: jq is required" >&2; exit 2; }
command -v flock >/dev/null 2>&1 || { echo "stale-db: flock is required" >&2; exit 2; }
[ -x "$GC" ] || command -v "$GC" >/dev/null 2>&1 || { echo "stale-db: gc is not executable: $GC" >&2; exit 2; }
mkdir -p "$REPORT_DIR"
exec 9>"$REPORT_DIR/run.lock"
flock -n 9 || { echo "stale-db: another maintenance run owns the lock"; exit 0; }

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mol-dog-stale-db.XXXXXX")"
trap 'rm -r "$TMP_DIR"' EXIT
SCAN_FILE="$TMP_DIR/scan.json"
APPLY_FILE="$TMP_DIR/apply.json"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
printf 'null\n' >"$APPLY_FILE"

gc() {
    env \
        -u BEADS_DIR \
        -u BEADS_DB \
        -u BEADS_DOLT_SERVER_HOST \
        -u BEADS_DOLT_SERVER_PORT \
        -u BEADS_DOLT_SERVER_USER \
        -u BEADS_DOLT_SERVER_PASSWORD \
        "$GC" --city "$CITY" "$@"
}

emit() {
    gc event emit "$1" --message "$2" >/dev/null 2>&1 || echo "stale-db: event emit failed: $1" >&2
}

notify_human() {
    gc mail send human -s "$1" -m "$2" >/dev/null 2>&1 || echo "stale-db: human notification failed" >&2
}

write_report() {
    local decision="$1"
    local detail="$2"
    local target="$REPORT_DIR/.$RUN_ID.json"
    jq -n \
        --arg schema "gc.maintenance.stale-db.v1" \
        --arg recorded_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --arg decision "$decision" \
        --arg detail "$detail" \
        --argjson max_orphans "$MAX_ORPHANS" \
        --argjson warn_threshold "$WARN_THRESHOLD" \
        --slurpfile scan "$SCAN_FILE" \
        --slurpfile apply "$APPLY_FILE" \
        '{schema:$schema, recorded_at:$recorded_at, decision:$decision, detail:$detail,
          thresholds:{max_orphans:$max_orphans,warn:$warn_threshold},
          scan:($scan[0] // null), apply:($apply[0] // null)}' >"$target"
    chmod 600 "$target"
    mv -f "$target" "$REPORT_DIR/$RUN_ID.json"
    cp "$REPORT_DIR/$RUN_ID.json" "$REPORT_DIR/.latest.$$.json"
    mv -f "$REPORT_DIR/.latest.$$.json" "$REPORT_DIR/latest.json"
}

write_raw_failure_report() {
    local decision="$1"
    local detail="$2"
    local target="$REPORT_DIR/.$RUN_ID.json"
    jq -n \
        --arg recorded_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --arg decision "$decision" \
        --arg detail "$detail" \
        --rawfile scan_raw "$SCAN_FILE" \
        --rawfile apply_raw "$APPLY_FILE" \
        '{schema:"gc.maintenance.stale-db.v1", recorded_at:$recorded_at,
          decision:$decision, detail:$detail, scan_raw:$scan_raw,
          apply_raw:(if $apply_raw == "null\n" then null else $apply_raw end)}' >"$target"
    chmod 600 "$target"
    mv -f "$target" "$REPORT_DIR/$RUN_ID.json"
    cp "$REPORT_DIR/$RUN_ID.json" "$REPORT_DIR/.latest.$$.json"
    mv -f "$REPORT_DIR/.latest.$$.json" "$REPORT_DIR/latest.json"
}

if ! gc dolt-cleanup --json --probe >"$SCAN_FILE"; then
    write_raw_failure_report "scan-failed" "gc dolt-cleanup probe exited nonzero"
    emit mol-dog-stale-db.escalate "dry-run failed before a cleanup decision"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "Dolt stale-db dry-run failed. Evidence: $REPORT_DIR/latest.json"
    exit 1
fi
if ! jq -e '.schema == "gc.dolt.cleanup.v1"' "$SCAN_FILE" >/dev/null 2>&1; then
    write_raw_failure_report "scan-invalid" "probe returned an invalid envelope"
    emit mol-dog-stale-db.escalate "dry-run returned invalid JSON"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "Dolt stale-db dry-run returned invalid JSON. Evidence: $REPORT_DIR/latest.json"
    exit 1
fi

ORPHAN_DBS="$(jq -r '.dropped.count // 0' "$SCAN_FILE")"
ORPHAN_PROCS="$(jq -r '.reaped.targets | length' "$SCAN_FILE")"
ORPHAN_TOTAL=$((ORPHAN_DBS + ORPHAN_PROCS))
DISK_BYTES="$(jq -r '.summary.bytes_freed_disk // .purge.bytes_reclaimed // 0' "$SCAN_FILE")"
RSS_BYTES="$(jq -r '.summary.bytes_freed_rss // 0' "$SCAN_FILE")"
SCAN_ERRS="$(jq -r '.summary.errors_total // 0' "$SCAN_FILE")"
INVALID_IDS="$(jq -r '[.dropped.skipped[]? | select(.reason == "invalid-identifier")] | length' "$SCAN_FILE")"
FORCE_BLOCKERS="$(jq -r '[.force_blockers[]?] | length' "$SCAN_FILE")"
for numeric in "$ORPHAN_DBS" "$ORPHAN_PROCS" "$DISK_BYTES" "$RSS_BYTES" "$SCAN_ERRS" "$INVALID_IDS" "$FORCE_BLOCKERS"; do
    case "$numeric" in ''|*[!0-9]*)
        write_report "scan-invalid" "probe returned a non-numeric counter"
        emit mol-dog-stale-db.escalate "dry-run returned a non-numeric counter"
        exit 1
        ;;
    esac
done

emit mol-dog-stale-db.scan "$ORPHAN_DBS stale databases, $ORPHAN_PROCS orphan processes, $SCAN_ERRS errors"

if [ "$SCAN_ERRS" -gt 0 ] || [ "$INVALID_IDS" -gt 0 ] || [ "$FORCE_BLOCKERS" -gt 0 ]; then
    detail="probe reported errors=$SCAN_ERRS invalid_identifiers=$INVALID_IDS force_blockers=$FORCE_BLOCKERS"
    write_report "scan-refused" "$detail"
    emit mol-dog-stale-db.escalate "$detail"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "$detail. Evidence: $REPORT_DIR/latest.json"
    exit 1
fi

if [ "$ORPHAN_TOTAL" -eq 0 ] && [ "$DISK_BYTES" -le 0 ]; then
    write_report "no-work" "no stale databases, orphan processes, or reclaimable bytes"
    emit mol-dog-stale-db.done "no stale Dolt work"
    exit 0
fi

if [ "$ORPHAN_DBS" -gt "$MAX_ORPHANS" ]; then
    detail="$ORPHAN_DBS stale databases exceed automatic limit $MAX_ORPHANS"
    write_report "threshold-escalated" "$detail"
    emit mol-dog-stale-db.escalate "$detail"
    emit mol-dog-stale-db.done "$detail; no mutation attempted"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "$detail. Evidence: $REPORT_DIR/latest.json"
    exit 0
fi

if ! gc dolt-cleanup --json --probe --force --max-orphan-dbs "$MAX_ORPHANS" >"$APPLY_FILE"; then
    if jq -e . "$APPLY_FILE" >/dev/null 2>&1; then
        write_report "apply-failed" "gc dolt-cleanup apply exited nonzero"
    else
        write_raw_failure_report "apply-failed" "gc dolt-cleanup apply exited nonzero with an invalid envelope"
    fi
    emit mol-dog-stale-db.escalate "apply failed; operator review required"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "Dolt cleanup apply failed. Do not retry automatically. Evidence: $REPORT_DIR/latest.json"
    exit 1
fi
if ! jq -e '.schema == "gc.dolt.cleanup.v1"' "$APPLY_FILE" >/dev/null 2>&1; then
    write_raw_failure_report "apply-invalid" "apply returned an invalid envelope"
    emit mol-dog-stale-db.escalate "apply returned invalid JSON"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "Dolt cleanup apply returned invalid JSON. Evidence: $REPORT_DIR/latest.json"
    exit 1
fi

APPLY_ERRS="$(jq -r '.summary.errors_total // 0' "$APPLY_FILE")"
case "$APPLY_ERRS" in ''|*[!0-9]*)
    write_report "apply-invalid" "apply returned a non-numeric error counter"
    emit mol-dog-stale-db.escalate "apply returned a non-numeric error counter"
    exit 1
    ;;
esac
if [ "$APPLY_ERRS" -gt 0 ]; then
    write_report "apply-failed" "apply envelope reported $APPLY_ERRS error(s)"
    emit mol-dog-stale-db.escalate "apply reported $APPLY_ERRS error(s)"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "Dolt cleanup apply reported errors. Do not retry automatically. Evidence: $REPORT_DIR/latest.json"
    exit 1
fi

DROP_OK="$(jq -r '.dropped.count // 0' "$APPLY_FILE")"
DROP_FAIL="$(jq -r '.dropped.failed | length' "$APPLY_FILE")"
PURGE_BYTES="$(jq -r '.purge.bytes_reclaimed // 0' "$APPLY_FILE")"
REAP_KILLED="$(jq -r '.reaped.count // 0' "$APPLY_FILE")"
REAP_TOTAL="$(jq -r '.reaped.targets | length' "$APPLY_FILE")"
for numeric in "$DROP_OK" "$DROP_FAIL" "$PURGE_BYTES" "$REAP_KILLED" "$REAP_TOTAL"; do
    case "$numeric" in ''|*[!0-9]*)
        write_report "apply-invalid" "apply returned a non-numeric counter"
        emit mol-dog-stale-db.escalate "apply returned a non-numeric counter"
        exit 1
        ;;
    esac
done
MISSED_PURGE_BYTES=0
if [ "$PURGE_BYTES" -lt "$DISK_BYTES" ]; then
    MISSED_PURGE_BYTES=$((DISK_BYTES - PURGE_BYTES))
fi

emit mol-dog-stale-db.drop "$DROP_OK/$ORPHAN_DBS dropped; $DROP_FAIL failed"
emit mol-dog-stale-db.purge "$PURGE_BYTES bytes reclaimed"
emit mol-dog-stale-db.reap "$REAP_KILLED/$REAP_TOTAL processes killed"

if [ "$MISSED_PURGE_BYTES" -gt 0 ]; then
    write_report "apply-incomplete" "$MISSED_PURGE_BYTES reclaimable bytes were not purged"
    emit mol-dog-stale-db.escalate "apply missed $MISSED_PURGE_BYTES reclaimable bytes"
    notify_human "NEEDS_OPERATOR dolt-maintenance" "Dolt cleanup left reclaimable bytes. Evidence: $REPORT_DIR/latest.json"
    exit 1
fi

write_report "applied" "cleanup completed: databases=$DROP_OK processes=$REAP_KILLED bytes=$((PURGE_BYTES + RSS_BYTES))"
emit mol-dog-stale-db.done "$((PURGE_BYTES + RSS_BYTES)) bytes freed; 0 errors"
if [ "$ORPHAN_TOTAL" -ge "$WARN_THRESHOLD" ]; then
    notify_human "MAINTENANCE_WARN dolt-maintenance" "$ORPHAN_TOTAL Dolt orphans were handled. Evidence: $REPORT_DIR/latest.json"
fi
