#!/usr/bin/env bash
# Sets up the functional test environment:
#   1. Creates a kind cluster
#   2. Builds and loads the operator image
#   3. Installs CRDs and deploys the controller
#   4. Deploys the in-cluster git server
#   5. Creates the test namespace
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib.sh"

KIND_CLUSTER="${KIND_CLUSTER:-ignition-sync-func-test}"
IMG="${IMG:-ignition-sync-operator:func-test}"
CONTROLLER_NS="ignition-sync-operator-system"

echo "=== Functional Test Setup ==="

# 1. Create kind cluster (if not exists)
echo "→ Checking kind cluster '${KIND_CLUSTER}'..."
if ! kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
    echo "  Creating kind cluster..."
    kind create cluster --name "${KIND_CLUSTER}" --wait 60s
else
    echo "  Cluster already exists."
fi

# Ensure kubectl context points to kind cluster
kubectl cluster-info --context "kind-${KIND_CLUSTER}" >/dev/null 2>&1

# 2. Build the operator image
echo "→ Building operator image '${IMG}'..."
cd "${PROJECT_ROOT}"
make docker-build IMG="${IMG}"

# 3. Load image into kind
echo "→ Loading image into kind..."
kind load docker-image "${IMG}" --name "${KIND_CLUSTER}"

# 4. Install CRDs
echo "→ Installing CRDs..."
make install

# 5. Deploy the controller
echo "→ Deploying controller..."
make deploy IMG="${IMG}"

# 6. Wait for controller-manager to be Running and Ready
echo "→ Waiting for controller-manager pod..."
deadline=$((SECONDS + 120))
while [[ $SECONDS -lt $deadline ]]; do
    phase=$(kubectl get pods -n "${CONTROLLER_NS}" -l control-plane=controller-manager \
        -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "")
    ready=$(kubectl get pods -n "${CONTROLLER_NS}" -l control-plane=controller-manager \
        -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
    if [[ "$phase" == "Running" && "$ready" == "True" ]]; then
        echo "  Controller-manager is Running and Ready."
        break
    fi
    sleep 3
done
if [[ "$phase" != "Running" || "$ready" != "True" ]]; then
    echo "ERROR: Controller-manager did not become ready within 120s"
    kubectl get pods -n "${CONTROLLER_NS}" -o wide 2>/dev/null || true
    kubectl logs -n "${CONTROLLER_NS}" -l control-plane=controller-manager --tail=50 2>/dev/null || true
    exit 1
fi

# 7. Create test namespace
echo "→ Creating test namespace '${TEST_NAMESPACE}'..."
setup_namespace "${TEST_NAMESPACE}"

# 8. Deploy in-cluster git server
echo "→ Deploying in-cluster git server..."
apply_fixture "git-server.yaml"

# 9. Wait for git server to be Running
echo "→ Waiting for git server pod..."
deadline=$((SECONDS + 90))
while [[ $SECONDS -lt $deadline ]]; do
    phase=$(kubectl get pod test-git-server -n "${TEST_NAMESPACE}" \
        -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
    ready=$(kubectl get pod test-git-server -n "${TEST_NAMESPACE}" \
        -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
    if [[ "$phase" == "Running" && "$ready" == "True" ]]; then
        echo "  Git server is Running and Ready."
        break
    fi
    sleep 3
done
if [[ "$phase" != "Running" || "$ready" != "True" ]]; then
    echo "ERROR: Git server did not become ready within 90s"
    kubectl describe pod test-git-server -n "${TEST_NAMESPACE}" 2>/dev/null || true
    exit 1
fi

echo ""
echo "=== Setup Complete ==="
echo "  Cluster:    ${KIND_CLUSTER}"
echo "  Namespace:  ${TEST_NAMESPACE}"
echo "  Image:      ${IMG}"
echo "  Git server: git://test-git-server.${TEST_NAMESPACE}.svc.cluster.local/test-repo.git"
