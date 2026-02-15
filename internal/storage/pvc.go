package storage

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	syncv1alpha1 "github.com/inductiveautomation/ignition-sync-operator/api/v1alpha1"
	synctypes "github.com/inductiveautomation/ignition-sync-operator/pkg/types"
)

// PVCName returns the deterministic PVC name for a given CR.
func PVCName(crName string) string {
	return fmt.Sprintf("ignition-sync-repo-%s", crName)
}

// EnsurePVC creates or verifies the shared repo PVC for an IgnitionSync CR.
// Returns the PVC and whether it was newly created.
func EnsurePVC(ctx context.Context, c client.Client, scheme *runtime.Scheme, isync *syncv1alpha1.IgnitionSync) (*corev1.PersistentVolumeClaim, bool, error) {
	pvcName := PVCName(isync.Name)
	existing := &corev1.PersistentVolumeClaim{}
	err := c.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: isync.Namespace}, existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.IsNotFound(err) {
		return nil, false, fmt.Errorf("checking PVC %s: %w", pvcName, err)
	}

	// Parse storage size
	size := isync.Spec.Storage.Size
	if size == "" {
		size = "1Gi"
	}
	qty, err := resource.ParseQuantity(size)
	if err != nil {
		return nil, false, fmt.Errorf("invalid storage size %q: %w", size, err)
	}

	// Determine access mode
	accessMode := corev1.ReadWriteMany
	if isync.Spec.Storage.AccessMode == "ReadWriteOnce" {
		accessMode = corev1.ReadWriteOnce
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: isync.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ignition-sync-controller",
				synctypes.LabelCRName:          isync.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: qty,
				},
			},
		},
	}

	// Set storage class if specified
	if isync.Spec.Storage.StorageClassName != "" {
		pvc.Spec.StorageClassName = &isync.Spec.Storage.StorageClassName
	}

	// Owner reference — GC cleans up PVC when CR is deleted
	if err := controllerutil.SetControllerReference(isync, pvc, scheme); err != nil {
		return nil, false, fmt.Errorf("setting owner reference on PVC: %w", err)
	}

	if err := c.Create(ctx, pvc); err != nil {
		return nil, false, fmt.Errorf("creating PVC %s: %w", pvcName, err)
	}

	return pvc, true, nil
}
