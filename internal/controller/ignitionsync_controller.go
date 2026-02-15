/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	syncv1alpha1 "github.com/inductiveautomation/ignition-sync-operator/api/v1alpha1"
	"github.com/inductiveautomation/ignition-sync-operator/internal/git"
	"github.com/inductiveautomation/ignition-sync-operator/internal/storage"
	"github.com/inductiveautomation/ignition-sync-operator/pkg/conditions"
	synctypes "github.com/inductiveautomation/ignition-sync-operator/pkg/types"
)

const (
	gitOpTimeout    = 2 * time.Minute
	gitPollInterval = 5 * time.Second
)

// gitOpResult stores the outcome of an async git operation.
type gitOpResult struct {
	commit string
	ref    string
	err    error
}

// gitOpState tracks an in-flight git operation.
type gitOpState struct {
	done   chan struct{}
	result gitOpResult
}

// IgnitionSyncReconciler reconciles an IgnitionSync object.
type IgnitionSyncReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	GitClient git.Client
	Recorder  record.EventRecorder
	gitOps    sync.Map // map[string]*gitOpState — keyed by "namespace/name"
}

// +kubebuilder:rbac:groups=sync.ignition.io,resources=ignitionsyncs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=sync.ignition.io,resources=ignitionsyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sync.ignition.io,resources=ignitionsyncs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *IgnitionSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR
	var isync syncv1alpha1.IgnitionSync
	if err := r.Get(ctx, req.NamespacedName, &isync); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// --- Step 0: Finalizer handling ---

	if !isync.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&isync, synctypes.Finalizer) {
			log.Info("cleaning up resources for deleted CR")
			if err := r.cleanupOwnedResources(ctx, &isync); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&isync, synctypes.Finalizer)
			return ctrl.Result{}, r.Update(ctx, &isync)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&isync, synctypes.Finalizer) {
		controllerutil.AddFinalizer(&isync, synctypes.Finalizer)
		return ctrl.Result{}, r.Update(ctx, &isync)
	}

	// --- Step 0.5: Check if paused ---

	if isync.Spec.Paused {
		log.Info("CR is paused, skipping reconciliation")
		r.setCondition(ctx, &isync, conditions.TypeReady, metav1.ConditionFalse, conditions.ReasonPaused, "Reconciliation paused")
		return ctrl.Result{}, r.patchStatus(ctx, &isync)
	}

	// --- Step 1: Validate secrets exist ---

	if err := r.validateSecrets(ctx, &isync); err != nil {
		r.setCondition(ctx, &isync, conditions.TypeReady, metav1.ConditionFalse, conditions.ReasonReconciling, err.Error())
		_ = r.patchStatus(ctx, &isync)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// --- Step 2: Ensure repo PVC ---

	pvc, created, err := storage.EnsurePVC(ctx, r.Client, r.Scheme, &isync)
	if err != nil {
		r.setCondition(ctx, &isync, conditions.TypeReady, metav1.ConditionFalse, conditions.ReasonReconciling, fmt.Sprintf("PVC error: %v", err))
		_ = r.patchStatus(ctx, &isync)
		return ctrl.Result{}, err
	}
	if created {
		log.Info("created repo PVC", "pvc", pvc.Name)
	}

	// --- Step 3: Clone or update repo (non-blocking) ---

	crKey := req.NamespacedName.String()
	result, requeue, err := r.handleGitOp(ctx, &isync, crKey)
	if err != nil {
		r.setCondition(ctx, &isync, conditions.TypeRepoCloned, metav1.ConditionFalse, conditions.ReasonCloneFailed, err.Error())
		isync.Status.RepoCloneStatus = "Error"
		_ = r.patchStatus(ctx, &isync)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if requeue {
		// Git op in progress — requeue to check again
		r.setCondition(ctx, &isync, conditions.TypeRepoCloned, metav1.ConditionFalse, conditions.ReasonSyncInProgress, "Git operation in progress")
		isync.Status.RepoCloneStatus = "Cloning"
		_ = r.patchStatus(ctx, &isync)
		return ctrl.Result{RequeueAfter: gitPollInterval}, nil
	}

	// Git op completed successfully
	r.setCondition(ctx, &isync, conditions.TypeRepoCloned, metav1.ConditionTrue, conditions.ReasonCloneSucceeded, result.Commit)
	isync.Status.RepoCloneStatus = "Cloned"
	isync.Status.LastSyncCommit = result.Commit
	isync.Status.LastSyncRef = result.Ref
	now := metav1.Now()
	isync.Status.LastSyncTime = &now

	// --- Step 4: Create/update metadata ConfigMap ---

	if err := r.ensureMetadataConfigMap(ctx, &isync, result); err != nil {
		log.Error(err, "failed to update metadata ConfigMap")
	}

	// --- Step 5: Discover gateways ---

	prevGatewayCount := len(isync.Status.DiscoveredGateways)
	gateways, err := r.discoverGateways(ctx, &isync)
	if err != nil {
		log.Error(err, "failed to discover gateways")
	} else {
		gateways = r.collectGatewayStatus(ctx, &isync, gateways)
		isync.Status.DiscoveredGateways = gateways

		if len(gateways) != prevGatewayCount {
			r.Recorder.Eventf(&isync, corev1.EventTypeNormal, "GatewaysDiscovered",
				"Discovered %d gateway(s) (was %d)", len(gateways), prevGatewayCount)
		}
	}

	// --- Step 6: Update conditions ---

	r.updateAllGatewaysSyncedCondition(ctx, &isync)
	r.updateReadyCondition(ctx, &isync)

	// --- Step 7: Update status ---

	isync.Status.ObservedGeneration = isync.Generation
	if err := r.patchStatus(ctx, &isync); err != nil {
		return ctrl.Result{}, err
	}

	// --- Step 8: Requeue ---

	requeueAfter := r.pollingInterval(&isync)
	log.Info("reconciliation complete", "commit", result.Commit, "gateways", len(isync.Status.DiscoveredGateways), "requeueAfter", requeueAfter)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// handleGitOp manages async git operations. Returns (result, needsRequeue, error).
func (r *IgnitionSyncReconciler) handleGitOp(ctx context.Context, isync *syncv1alpha1.IgnitionSync, crKey string) (git.Result, bool, error) {
	// Check for an existing in-flight operation
	if val, ok := r.gitOps.Load(crKey); ok {
		state := val.(*gitOpState)
		select {
		case <-state.done:
			// Operation finished — consume result and clean up
			r.gitOps.Delete(crKey)
			if state.result.err != nil {
				return git.Result{}, false, state.result.err
			}
			return git.Result{Commit: state.result.commit, Ref: state.result.ref}, false, nil
		default:
			// Still running
			return git.Result{}, true, nil
		}
	}

	// No in-flight operation — resolve auth and launch one
	auth, err := git.ResolveAuth(ctx, r.Client, isync.Namespace, isync.Spec.Git.Auth)
	if err != nil {
		return git.Result{}, false, fmt.Errorf("resolving git auth: %w", err)
	}

	pvcPath := fmt.Sprintf("/data/%s/%s", isync.Namespace, storage.PVCName(isync.Name))
	ref := isync.Spec.Git.Ref

	// Check for webhook-requested ref override
	if requested, ok := isync.Annotations[synctypes.AnnotationRequestedRef]; ok && requested != "" {
		ref = requested
	}

	state := &gitOpState{done: make(chan struct{})}
	r.gitOps.Store(crKey, state)

	go func() {
		defer close(state.done)
		gitCtx, cancel := context.WithTimeout(context.Background(), gitOpTimeout)
		defer cancel()

		result, err := r.GitClient.CloneOrFetch(gitCtx, isync.Spec.Git.Repo, ref, pvcPath, auth)
		state.result = gitOpResult{
			commit: result.Commit,
			ref:    result.Ref,
			err:    err,
		}
	}()

	// Just launched — requeue to check later
	return git.Result{}, true, nil
}

// cleanupOwnedResources removes ConfigMaps owned by this CR during deletion.
func (r *IgnitionSyncReconciler) cleanupOwnedResources(ctx context.Context, isync *syncv1alpha1.IgnitionSync) error {
	log := logf.FromContext(ctx)

	// Clean up metadata, status, and changes ConfigMaps
	cmNames := []string{
		fmt.Sprintf("ignition-sync-metadata-%s", isync.Name),
		fmt.Sprintf("ignition-sync-status-%s", isync.Name),
		fmt.Sprintf("ignition-sync-changes-%s", isync.Name),
	}

	for _, name := range cmNames {
		cm := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: isync.Namespace}, cm)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("getting ConfigMap %s: %w", name, err)
		}
		if err := r.Delete(ctx, cm); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting ConfigMap %s: %w", name, err)
		}
		log.Info("deleted ConfigMap", "name", name)
	}

	// Cancel any in-flight git operation
	crKey := fmt.Sprintf("%s/%s", isync.Namespace, isync.Name)
	r.gitOps.Delete(crKey)

	return nil
}

// validateSecrets checks that referenced secrets exist.
func (r *IgnitionSyncReconciler) validateSecrets(ctx context.Context, isync *syncv1alpha1.IgnitionSync) error {
	// Gateway API key secret is always required
	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      isync.Spec.Gateway.APIKeySecretRef.Name,
		Namespace: isync.Namespace,
	}
	if err := r.Get(ctx, key, secret); err != nil {
		return fmt.Errorf("gateway API key secret %q not found: %w", key.Name, err)
	}

	// Validate git auth secret if specified
	if isync.Spec.Git.Auth != nil {
		if isync.Spec.Git.Auth.SSHKey != nil {
			key.Name = isync.Spec.Git.Auth.SSHKey.SecretRef.Name
			if err := r.Get(ctx, key, secret); err != nil {
				return fmt.Errorf("SSH key secret %q not found: %w", key.Name, err)
			}
		}
		if isync.Spec.Git.Auth.Token != nil {
			key.Name = isync.Spec.Git.Auth.Token.SecretRef.Name
			if err := r.Get(ctx, key, secret); err != nil {
				return fmt.Errorf("token secret %q not found: %w", key.Name, err)
			}
		}
		if isync.Spec.Git.Auth.GitHubApp != nil {
			key.Name = isync.Spec.Git.Auth.GitHubApp.PrivateKeySecretRef.Name
			if err := r.Get(ctx, key, secret); err != nil {
				return fmt.Errorf("GitHub App private key secret %q not found: %w", key.Name, err)
			}
		}
	}

	return nil
}

// ensureMetadataConfigMap creates or updates the metadata ConfigMap that signals agents.
func (r *IgnitionSyncReconciler) ensureMetadataConfigMap(ctx context.Context, isync *syncv1alpha1.IgnitionSync, result git.Result) error {
	cmName := fmt.Sprintf("ignition-sync-metadata-%s", isync.Name)
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: cmName, Namespace: isync.Namespace}

	data := map[string]string{
		"commit":  result.Commit,
		"ref":     result.Ref,
		"trigger": time.Now().UTC().Format(time.RFC3339),
	}

	err := r.Get(ctx, key, cm)
	if errors.IsNotFound(err) {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: isync.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "ignition-sync-controller",
					synctypes.LabelCRName:          isync.Name,
				},
			},
			Data: data,
		}
		if err := controllerutil.SetControllerReference(isync, cm, r.Scheme); err != nil {
			return fmt.Errorf("setting owner ref on ConfigMap: %w", err)
		}
		return r.Create(ctx, cm)
	}
	if err != nil {
		return fmt.Errorf("getting ConfigMap %s: %w", cmName, err)
	}

	cm.Data = data
	return r.Update(ctx, cm)
}

// setCondition sets a condition on the CR's status.
func (r *IgnitionSyncReconciler) setCondition(_ context.Context, isync *syncv1alpha1.IgnitionSync, condType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: isync.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Replace existing condition of same type, or append
	for i, c := range isync.Status.Conditions {
		if c.Type == condType {
			if c.Status != status {
				isync.Status.Conditions[i] = condition
			} else {
				// Update reason/message but keep transition time
				isync.Status.Conditions[i].Reason = reason
				isync.Status.Conditions[i].Message = message
				isync.Status.Conditions[i].ObservedGeneration = isync.Generation
			}
			return
		}
	}
	isync.Status.Conditions = append(isync.Status.Conditions, condition)
}

// patchStatus applies a status update via merge patch.
func (r *IgnitionSyncReconciler) patchStatus(ctx context.Context, isync *syncv1alpha1.IgnitionSync) error {
	return r.Status().Update(ctx, isync)
}

// pollingInterval returns the requeue interval from the CR spec.
func (r *IgnitionSyncReconciler) pollingInterval(isync *syncv1alpha1.IgnitionSync) time.Duration {
	if isync.Spec.Polling.Enabled != nil && !*isync.Spec.Polling.Enabled {
		return 0 // no requeue if polling disabled
	}
	interval := isync.Spec.Polling.Interval
	if interval == "" {
		interval = "60s"
	}
	d, err := time.ParseDuration(interval)
	if err != nil {
		return 60 * time.Second
	}
	return d
}

// SetupWithManager sets up the controller with the Manager.
func (r *IgnitionSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syncv1alpha1.IgnitionSync{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.findIgnitionSyncForPod)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		Named("ignitionsync").
		Complete(r)
}
