#!/usr/bin/env bash
#
# Verifies the local toolchain before any other task runs.
# Exits non-zero if a hard requirement is missing or too old; soft requirements
# (only needed for the mobile app) are reported as warnings.

set -uo pipefail

failures=0
warnings=0

red()   { printf '\033[31m%s\033[0m\n' "$1"; }
green() { printf '\033[32m%s\033[0m\n' "$1"; }
amber() { printf '\033[33m%s\033[0m\n' "$1"; }

# Returns 0 when $1 >= $2 using dotted-version ordering.
version_ge() {
  [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
}

# check <label> <command> <version-command> <minimum> <hard|soft>
check() {
  local label=$1 cmd=$2 version_cmd=$3 minimum=$4 severity=$5

  if ! command -v "$cmd" >/dev/null 2>&1; then
    if [ "$severity" = hard ]; then
      red   "FAIL  $label is not installed (need >= $minimum)"
      failures=$((failures + 1))
    else
      amber "WARN  $label is not installed (need >= $minimum, required for the mobile app)"
      warnings=$((warnings + 1))
    fi
    return
  fi

  local found
  found=$(eval "$version_cmd" 2>&1 | tr -d '\r' | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n1)

  if [ -z "$found" ]; then
    amber "WARN  $label is installed but its version could not be parsed"
    warnings=$((warnings + 1))
    return
  fi

  if version_ge "$found" "$minimum"; then
    green "OK    $label $found (>= $minimum)"
  elif [ "$severity" = hard ]; then
    red   "FAIL  $label $found is too old (need >= $minimum)"
    failures=$((failures + 1))
  else
    amber "WARN  $label $found is too old (need >= $minimum)"
    warnings=$((warnings + 1))
  fi
}

echo "ParkXchange toolchain check"
echo "---------------------------"

# Backend and orchestration.
check "Go"     go     "go version"        1.24.0  hard
check "Task"   task   "task --version"    3.40.0  hard
check "Docker" docker "docker --version"  24.0.0  hard

# JS/TS ecosystem. Expo SDK 57 (React Native 0.86) requires Node >= 22.13.
check "Node"   node   "node --version"    22.13.0 hard
check "pnpm"   pnpm   "pnpm --version"    10.0.0  hard

# Android build chain. Only needed from the mobile phase onwards.
check "Java"   java   "java -version"     17.0.0  soft

if [ -n "${ANDROID_HOME:-}" ] && [ -d "${ANDROID_HOME:-}" ]; then
  green "OK    ANDROID_HOME $ANDROID_HOME"
else
  amber "WARN  ANDROID_HOME is unset or missing (required for the mobile app)"
  warnings=$((warnings + 1))
fi

# A running daemon is only needed for the database tasks, so this is a warning.
if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then
    green "OK    Docker daemon is running"
  else
    amber "WARN  Docker daemon is not running (start Docker Desktop before 'task db:up')"
    warnings=$((warnings + 1))
  fi
fi

echo "---------------------------"
if [ "$failures" -gt 0 ]; then
  red "$failures hard requirement(s) unmet, $warnings warning(s)."
  exit 1
fi

if [ "$warnings" -gt 0 ]; then
  amber "All hard requirements met, $warnings warning(s)."
else
  green "All checks passed."
fi
