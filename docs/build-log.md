# Ignition Sync Operator — Build Log

Tracks each implementation phase, its steps, and status.

---

## Phase 0: Scaffolding & Foundation

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Install Go and kubebuilder | done | Go 1.26.0, kubebuilder 4.11.1 via Homebrew |
| 2 | Initialize git repo | done | `git init`, `.gitignore` created |
| 3 | Scaffold kubebuilder project | done | `kubebuilder init --domain ignition.io --repo github.com/inductiveautomation/ignition-sync-operator` |
| 4 | Create API scaffolding | done | `kubebuilder create api --group sync --version v1alpha1 --kind IgnitionSync --resource --controller` |
| 5 | Restructure to multi-binary layout | done | `cmd/main.go` → `cmd/controller/main.go`, created `cmd/agent/main.go` stub |
| 6 | Update Makefile for dual binaries | done | `build` target builds both `bin/manager` and `bin/agent` |
| 7 | Create Dockerfile.agent | done | `gcr.io/distroless/static-debian12:nonroot`, uid 65534 |
| 8 | Update Dockerfile for controller path | done | `cmd/main.go` → `cmd/controller/main.go` |
| 9 | Create `pkg/types/annotations.go` | done | 13 pod annotations, 3 CR annotations, 1 label, 1 finalizer |
| 10 | Create `pkg/conditions/conditions.go` | done | 5 condition types, 8 reason constants |
| 11 | Verify `make generate` | done | DeepCopy generated |
| 12 | Verify `make manifests` | done | CRD YAML at `config/crd/bases/sync.ignition.io_ignitionsyncs.yaml` |
| 13 | Verify `make test` | done | envtest passes, controller 66.7% coverage |
| 14 | Verify `make build` | done | Both `bin/manager` and `bin/agent` compile |

**Key files created:**
- `cmd/controller/main.go` — controller-runtime manager entrypoint
- `cmd/agent/main.go` — agent stub (Phase 5 fills in)
- `api/v1alpha1/ignitionsync_types.go` — scaffold types (Phase 1 replaces)
- `api/v1alpha1/groupversion_info.go` — group/version registration
- `internal/controller/ignitionsync_controller.go` — empty reconciler (Phase 2 fills in)
- `pkg/types/annotations.go` — annotation/label/finalizer constants
- `pkg/conditions/conditions.go` — condition type/reason constants
- `Dockerfile` — controller image
- `Dockerfile.agent` — agent image
- `Makefile` — build, test, generate, deploy targets

---

## Phase 1: CRD Types & Validation

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Write full CRD types | done | 27 structs in `api/v1alpha1/ignitionsync_types.go` |
| 2 | Add kubebuilder validation markers | done | Required, MinLength, Enum on key fields |
| 3 | Add kubebuilder default markers | done | 1Gi, ReadWriteMany, 8043, 8443, 60s, true, gitWins, wait, etc. |
| 4 | Add print column markers | done | Ref, Synced, Gateways, Ready, Age |
| 5 | Add short name + storageversion markers | done | `isync`, `igs`, `+kubebuilder:storageversion` |
| 6 | Run `make generate` | done | 826-line DeepCopy generated (up from 128) |
| 7 | Run `make manifests` | done | 861-line CRD YAML with all defaults/enums/columns |
| 8 | Update sample CR | done | Minimal CR with git + gateway.apiKeySecretRef |
| 9 | Fix scaffold test for new required fields | done | Added Git + Gateway specs to test resource |
| 10 | Verify `make build` | done | Both binaries compile |
| 11 | Verify `make test` | done | envtest passes, validation enforced at CRD level |

**Structs defined (spec):**
SecretKeyRef, GitSpec, GitAuthSpec, SSHKeyAuth, GitHubAppAuth, TokenAuth, StorageSpec, WebhookSpec, PollingSpec, GatewaySpec, SharedSpec, ExternalResourcesSpec, ScriptsSpec, UDTsSpec, AdditionalFile, NormalizeSpec, FieldReplacement, BidirectionalSpec, BidirectionalGuardrailsSpec, ValidationSpec, ValidationWebhookSpec, SnapshotSpec, SnapshotStorageSpec, S3StorageSpec, DeploymentStrategySpec, DeploymentStage, AutoRollbackSpec, IgnitionSpec, AgentSpec, AgentImageSpec

**Structs defined (status):**
DiscoveredGateway, GatewaySnapshot, SyncHistoryEntry

**Design decisions:**
- Experimental fields (Bidirectional, Snapshots, Deployment) are pointer types for nil-vs-zero distinction
- `corev1.ResourceRequirements` used for agent resources (copies directly to container spec)
- Custom `SecretKeyRef` instead of `corev1.SecretKeySelector` to keep CRD schema lean
- Validation is CRD-level (OpenAPI schema), not webhook-based

---

## Phase 2: Controller Core — PVC, Git & Finalizer

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Add go-git dependency | done | `go-git/v5 v5.16.5` |
| 2 | Implement `internal/storage/pvc.go` | done | EnsurePVC with owner ref, labels, storage class |
| 3 | Implement `internal/git/auth.go` | done | SSH key + HTTPS token auth from Secrets |
| 4 | Implement `internal/git/client.go` | done | CloneOrFetch via go-git, ref resolution (SHA/tag/branch) |
| 5 | Implement controller reconcile loop | done | Steps 0-6: finalizer, validation, PVC, non-blocking git, ConfigMap, conditions, requeue |
| 6 | Add RBAC markers | done | PVCs, ConfigMaps, Secrets (read), Events |
| 7 | Update `cmd/controller/main.go` | done | Inject GoGitClient |
| 8 | Write tests | done | Finalizer, PVC, git lifecycle, error handling, paused CR, secret validation |
| 9 | Verify `make generate` + `make manifests` | done | RBAC YAML regenerated with new permissions |
| 10 | Verify `make build` | done | Both binaries compile |
| 11 | Verify `make test` | done | envtest passes, controller 77.7% coverage |

**Key files created/modified:**
- `internal/storage/pvc.go` — PVC creation with owner reference + labels
- `internal/git/client.go` — Git client interface + go-git implementation (clone, fetch, checkout, ref resolution)
- `internal/git/auth.go` — SSH key and HTTPS token auth from K8s Secrets
- `internal/controller/ignitionsync_controller.go` — Full reconcile loop (finalizer, validation, PVC, async git, ConfigMap, conditions)
- `cmd/controller/main.go` — Wired GoGitClient into reconciler
- `internal/controller/ignitionsync_controller_test.go` — 6 test cases covering full lifecycle
- `config/rbac/role.yaml` — Auto-generated RBAC with PVC/ConfigMap/Secret/Event permissions

**Design decisions:**
- Non-blocking git via `sync.Map` + goroutine: first reconcile launches clone, subsequent reconciles check result
- Git operations have 2m context timeout; controller requeues every 5s while git is in flight
- `GenerationChangedPredicate` on primary watch prevents status-update reconcile storms
- `MaxConcurrentReconciles: 5` so one slow clone doesn't block other CRs
- Controller owns PVCs and ConfigMaps (garbage collected on CR deletion)
- Finalizer explicitly cleans ConfigMaps (metadata, status, changes); PVC is GC'd by owner ref
- Webhook-requested ref override via annotation takes precedence over spec.git.ref
- GitHub App auth stubbed but not implemented (deferred)

---

## Phase 3: ConfigMap Signaling & Gateway Discovery

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Create `pkg/types/sync_status.go` | done | GatewayStatus struct + 4 sync status constants |
| 2 | Create `internal/controller/gateway_discovery.go` | done | 5 methods: findIgnitionSyncForPod, discoverGateways, collectGatewayStatus, updateAllGatewaysSyncedCondition, updateReadyCondition |
| 3 | Wire discovery + events into controller | done | Steps 5-6 in reconcile loop, pod watch, EventRecorder, pods RBAC |
| 4 | Write Phase 3 tests | done | 7 new test cases covering discovery, status, conditions, pod mapping |
| 5 | Verify `make generate` + `make manifests` | done | Pods RBAC added to role.yaml |
| 6 | Verify `make build` | done | Both binaries compile |
| 7 | Verify `make test` | done | 17 tests pass, controller 82.5% coverage |

**Key files created/modified:**
- `pkg/types/sync_status.go` — GatewayStatus JSON schema + sync status constants (Pending/Syncing/Synced/Error)
- `internal/controller/gateway_discovery.go` — Pod-to-CR mapping, gateway discovery, status collection from ConfigMap, condition aggregation
- `internal/controller/ignitionsync_controller.go` — Added steps 5-6 (discover gateways, update conditions), pod watch via `handler.EnqueueRequestsFromMapFunc`, EventRecorder, pods RBAC marker
- `cmd/controller/main.go` — Passes `mgr.GetEventRecorderFor` to reconciler
- `internal/controller/ignitionsync_controller_test.go` — 7 new tests: discovery, multi-CR isolation, name fallback, status enrichment, Ready=True, partial sync, no gateways, pod mapping
- `config/rbac/role.yaml` — Auto-generated with pods get/list/watch

**Design decisions:**
- `GenerationChangedPredicate` moved from global `WithEventFilter` to `For()` only — pods and ConfigMaps need unrestricted watch events
- Gateway status read from `ignition-sync-status-{crName}` ConfigMap (written by agents, not controller-owned)
- Gateway name resolution: annotation → label `app.kubernetes.io/name` → pod name
- Ready condition is an AND of RepoCloned + AllGatewaysSynced
- Events emitted only on gateway count changes (lightweight, deduplicated by API server)

---

## Phase 4: Webhook Receiver

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Create `internal/webhook/hmac.go` | done | HMAC-SHA256 with `crypto/subtle.ConstantTimeCompare` |
| 2 | Create `internal/webhook/receiver.go` | done | HTTP handler, 4 payload formats, CR annotation, `manager.Runnable` |
| 3 | Wire webhook into `cmd/controller/main.go` | done | `--webhook-receiver-port` flag, `WEBHOOK_HMAC_SECRET` env var |
| 4 | Write HMAC tests | done | 4 tests: valid, invalid, missing prefix, empty secret |
| 5 | Write receiver tests | done | 12 tests: payload parsing (4 formats + empty + invalid), HTTP handler (accept, 404, 401, valid HMAC, annotation, duplicate, bad payload, HMAC-before-lookup) |
| 6 | Verify `make build` | done | Both binaries compile |
| 7 | Verify `make test` | done | Controller 82.5%, webhook 75.0% |

**Key files created/modified:**
- `internal/webhook/hmac.go` — HMAC-SHA256 validation with constant-time comparison
- `internal/webhook/receiver.go` — HTTP server implementing `manager.Runnable`, auto-detects GitHub/ArgoCD/Kargo/Generic payloads, annotates CR with requested-ref/at/by
- `internal/webhook/hmac_test.go` — 4 HMAC unit tests
- `internal/webhook/receiver_test.go` — 12 tests: payload parsing + HTTP handler with fake k8s client
- `cmd/controller/main.go` — Registers webhook receiver as `mgr.Add(Runnable)`, flag for port, env var for HMAC secret

**Design decisions:**
- Standard library `net/http` routing (Go 1.22+ patterns: `POST /webhook/{namespace}/{crName}`)
- Implements `manager.Runnable` for lifecycle management (starts/stops with controller-runtime manager)
- Global HMAC secret via `WEBHOOK_HMAC_SECRET` env var — validated before any CR lookup to prevent enumeration
- Per-CR `spec.webhook.secretRef` deferred (chicken-and-egg: can't read per-CR secret before CR lookup)
- Annotation-based trigger (not spec mutation) — avoids conflicts with GitOps controllers like ArgoCD
- Duplicate ref requests return 200 OK (idempotent), new refs return 202 Accepted
- 1 MiB payload limit via `io.LimitReader`

---

## Phase 3a: SyncProfile CRD

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Define SyncProfile types | done | SyncProfileSpec, SyncMapping, DeploymentModeSpec, ProfileDependency, DryRunDiffSummary |
| 2 | Generate CRD manifest | done | `sync.ignition.io_syncprofiles.yaml` with printer columns (Mode, Gateways, Accepted, Age) |
| 3 | Implement SyncProfile controller | done | Validates mappings (path traversal, absolute paths), sets Accepted condition |
| 4 | Integrate with gateway discovery | done | Reads `sync-profile` annotation, populates `SyncProfile` on DiscoveredGateway, `updateProfileGatewayCounts` |
| 5 | Wire into IgnitionSync controller | done | SyncProfile watch enqueues IgnitionSync reconciles via label selector |
| 6 | Register in main.go | done | SyncProfileReconciler registered alongside IgnitionSyncReconciler |
| 7 | Update RBAC | done | syncprofiles get/list/patch/update/watch, syncprofiles/status get/patch/update |
| 8 | Update conditions package | done | TypeAccepted, ReasonValidationPassed, ReasonValidationFailed |
| 9 | Write unit tests | done | 5 tests: valid profile, path traversal, absolute path, deploymentMode traversal, optional fields roundtrip |
| 10 | Verify `make test` | done | 20/20 tests pass (5 new + 15 existing) |
| 11 | Lab 03a manual verification | done | 16/16 checks pass in kind cluster |

**Key files created/modified:**
- `api/v1alpha1/syncprofile_types.go` — SyncProfile CRD types (SyncProfileSpec, SyncMapping, DeploymentModeSpec, ProfileDependency, DryRunDiffSummary)
- `config/crd/bases/sync.ignition.io_syncprofiles.yaml` — Generated CRD with printer columns and validation
- `internal/controller/syncprofile_controller.go` — Validates spec, sets Accepted condition, watches SyncProfile
- `internal/controller/syncprofile_controller_test.go` — 5 envtest-based unit tests
- `internal/controller/gateway_discovery.go` — Added SyncProfile field to DiscoveredGateway, `updateProfileGatewayCounts`
- `internal/controller/ignitionsync_controller.go` — SyncProfile watch in SetupWithManager, step 4.5 profile count update
- `api/v1alpha1/ignitionsync_types.go` — Added `SyncProfile` field to DiscoveredGateway
- `cmd/controller/main.go` — Registered SyncProfileReconciler
- `pkg/conditions/conditions.go` — Added TypeAccepted, ReasonValidationPassed, ReasonValidationFailed
- `config/rbac/role.yaml` — Auto-generated with syncprofiles permissions

**Lab 03a results (kind cluster, 16/16 checks):**
- 3A.1: CRD installed, short name `sp` works
- 3A.2: Valid profile → Accepted=True, observedGeneration=1
- 3A.3: Path traversal → Accepted=False
- 3A.4: Absolute path → Accepted=False
- 3A.5–3A.7: Gateway discovery with sync-profile annotation, gatewayCount updates
- 3A.8–3A.9: Profile update triggers reconcile, deletion degrades gracefully
- 3A.10, 3A.12–3A.15: Optional fields (paused, dependsOn, vars, dryRun, required) roundtrip correctly
- 3A.16: ref-override annotation preserved on discovered gateways
- 3A.17: Ignition health check passes, operator 0 restarts, 0 errors

**Design decisions:**
- SyncProfile is a passive CRD — controller validates spec and sets Accepted condition, no reconciliation loop beyond validation
- 3-tier config model: IgnitionSync (git, webhook, polling) → SyncProfile (mappings, deploymentMode, vars) → Pod annotations
- Cross-controller: IgnitionSync controller patches `gatewayCount` on SyncProfile status during gateway discovery
- Path validation rejects `..` traversal and absolute paths in both mappings and deploymentMode.source

---

## Phase 5: Sync Agent

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Agent binary (cmd/agent/main.go) | pending | |
| 2 | ConfigMap watcher | pending | |
| 3 | File copy from PVC to /ignition-data/ | pending | |
| 4 | Config normalization (systemName) | pending | |
| 5 | Ignition scan API integration | pending | |
| 6 | .resources/ protection | pending | |
| 7 | Tests | pending | |

---

## Phase 6: Sidecar Injection Webhook

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Mutating admission webhook | pending | |
| 2 | Inject agent + shared PVC volume | pending | |
| 3 | Cert management | pending | |
| 4 | Tests | pending | |

---

## Phase 7: Helm Chart

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Chart scaffolding | pending | |
| 2 | Values + templates | pending | |
| 3 | RBAC + ServiceAccount | pending | |
| 4 | Tests (helm template) | pending | |

---

## Phase 8: Observability & Polish

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Prometheus metrics | pending | |
| 2 | Structured logging | pending | |
| 3 | E2E tests in kind | pending | |
| 4 | Documentation | pending | |
