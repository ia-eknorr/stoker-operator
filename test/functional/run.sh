#!/usr/bin/env bash
# Orchestrator for functional tests.
# Usage:
#   ./run.sh           # Run all phases
#   ./run.sh 02        # Run only phase 02
#   ./run.sh 02 03     # Run phases 02 and 03
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Determine which phases to run
PHASES=("$@")
if [[ ${#PHASES[@]} -eq 0 ]]; then
    # Auto-detect available phase scripts
    PHASES=()
    for f in "${SCRIPT_DIR}"/phase-*.sh; do
        if [[ -f "$f" ]]; then
            phase=$(basename "$f" | sed -E 's/phase-([0-9a-z]+)-.*/\1/')
            PHASES+=("$phase")
        fi
    done
fi

if [[ ${#PHASES[@]} -eq 0 ]]; then
    echo "No phase scripts found."
    exit 1
fi

echo "═══════════════════════════════════════════════════"
echo "  Functional Test Runner"
echo "  Phases: ${PHASES[*]}"
echo "═══════════════════════════════════════════════════"

# Setup
echo ""
echo ">>> Running setup..."
bash "${SCRIPT_DIR}/setup.sh"

# Trap to ensure teardown runs
cleanup() {
    local exit_code=$?
    echo ""
    echo ">>> Running teardown..."
    bash "${SCRIPT_DIR}/teardown.sh" || true
    exit $exit_code
}
trap cleanup EXIT

# Run each phase
OVERALL_RESULT=0
for phase in "${PHASES[@]}"; do
    # Find the matching script
    phase_script=$(ls "${SCRIPT_DIR}"/phase-"${phase}"-*.sh 2>/dev/null | head -1)
    if [[ -z "$phase_script" || ! -f "$phase_script" ]]; then
        echo "ERROR: No script found for phase ${phase}"
        OVERALL_RESULT=1
        break
    fi

    echo ""
    echo ">>> Running $(basename "$phase_script")..."
    if ! bash "$phase_script"; then
        echo ""
        echo "ERROR: Phase ${phase} FAILED — stopping."
        OVERALL_RESULT=1
        break
    fi
done

if [[ $OVERALL_RESULT -eq 0 ]]; then
    echo ""
    echo "═══════════════════════════════════════════════════"
    echo "  ALL PHASES PASSED"
    echo "═══════════════════════════════════════════════════"
fi

# Exit code propagated by trap
exit $OVERALL_RESULT
