package snapshot

import (
	"context"

	snapshot "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	snapshotclient "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"

	"github.com/kanisterio/kanister/pkg/poll"
)

type snapshotAlpha struct {
}

func (sna *snapshotAlpha) GetVolumeSnapshotClass(annotation string, snapCli snapshotclient.Interface) (string, error) {
	vscl, err := snapCli.VolumesnapshotV1alpha1().VolumeSnapshotClasses().List(metav1.ListOptions{})
	if err != nil {
		return "", errors.Wrap(err, "Failed to get VolumeSnapshotClasses in the cluster")
	}
	if len(vscl.Items) == 0 {
		return "", errors.Wrap(err, "Failed to find any VolumeSnapshotClass in the cluster")
	}
	for _, vsc := range vscl.Items {
		if val, ok := vsc.ObjectMeta.Annotations[annotation]; ok && val == "true" {
			return vsc.GetName(), nil
		}
	}
	return "", errors.New("Failed to find VolumesnapshotClass with K10 annotation in the cluster")

}

func (sna *snapshotAlpha) Create(ctx context.Context, kubeCli kubernetes.Interface, snapCli snapshotclient.Interface, name, namespace, volumeName string, snapshotClass *string, waitForReady bool) error {
	if _, err := kubeCli.CoreV1().PersistentVolumeClaims(namespace).Get(volumeName, metav1.GetOptions{}); err != nil {
		if k8errors.IsNotFound(err) {
			return errors.Errorf("Failed to find PVC %s, Namespace %s", volumeName, namespace)
		}
		return errors.Errorf("Failed to query PVC %s, Namespace %s: %v", volumeName, namespace, err)
	}

	snap := &snapshot.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: snapshot.VolumeSnapshotSpec{
			Source: &corev1.TypedLocalObjectReference{
				Kind: pvcKind,
				Name: volumeName,
			},
			VolumeSnapshotClassName: snapshotClass,
		},
	}

	_, err := snapCli.VolumesnapshotV1alpha1().VolumeSnapshots(namespace).Create(snap)
	if err != nil {
		return err
	}

	if !waitForReady {
		return nil
	}

	err = sna.WaitOnReadyToUse(ctx, snapCli, name, namespace)
	if err != nil {
		return err
	}

	_, err = sna.Get(ctx, snapCli, name, namespace)
	return err
}

func (sna *snapshotAlpha) GetSource(ctx context.Context, snapCli snapshotclient.Interface, snapshotName, namespace string) (*Source, error) {
	snap, err := sna.Get(ctx, snapCli, snapshotName, namespace)
	if err != nil {
		return nil, errors.Errorf("Failed to get snapshot, VolumeSnapshot: %s, Error: %v", snapshotName, err)
	}
	cont, err := sna.getContent(ctx, snapCli, snap.Spec.SnapshotContentName)
	if err != nil {
		return nil, errors.Errorf("Failed to get snapshot content, VolumeSnapshot: %s, VolumeSnapshotContent: %s, Error: %v", snapshotName, snap.Spec.SnapshotContentName, err)
	}
	src := &Source{
		Handle:                  cont.Spec.CSI.SnapshotHandle,
		Driver:                  cont.Spec.CSI.Driver,
		RestoreSize:             cont.Spec.CSI.RestoreSize,
		VolumeSnapshotClassName: cont.Spec.VolumeSnapshotClassName,
	}
	return src, nil
}

func (sna *snapshotAlpha) CreateFromSource(ctx context.Context, snapCli snapshotclient.Interface, source *Source, snapshotName, namespace string, waitForReady bool) error {
	deletionPolicy, err := sna.getDeletionPolicyFromClass(snapCli, *source.VolumeSnapshotClassName)
	if err != nil {
		return errors.Wrap(err, "Failed to get DeletionPolicy from VolumeSnapshotClass")
	}
	contentName := snapshotName + "-content-" + string(uuid.NewUUID())
	content := &snapshot.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: snapshot.VolumeSnapshotContentSpec{
			VolumeSnapshotSource: snapshot.VolumeSnapshotSource{
				CSI: &snapshot.CSIVolumeSnapshotSource{
					Driver:         source.Driver,
					SnapshotHandle: source.Handle,
				},
			},
			VolumeSnapshotRef: &corev1.ObjectReference{
				Kind:      snapshotKind,
				Namespace: namespace,
				Name:      snapshotName,
			},
			VolumeSnapshotClassName: source.VolumeSnapshotClassName,
			DeletionPolicy:          deletionPolicy,
		},
	}
	snap := &snapshot.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: snapshotName,
		},
		Spec: snapshot.VolumeSnapshotSpec{
			SnapshotContentName:     content.Name,
			VolumeSnapshotClassName: content.Spec.VolumeSnapshotClassName,
		},
	}

	content, err = snapCli.VolumesnapshotV1alpha1().VolumeSnapshotContents().Create(content)
	if err != nil {
		return errors.Errorf("Failed to create content, VolumesnapshotContent: %s, Error: %v", content.Name, err)
	}
	snap, err = snapCli.VolumesnapshotV1alpha1().VolumeSnapshots(namespace).Create(snap)
	if err != nil {
		return errors.Errorf("Failed to create content, Volumesnapshot: %s, Error: %v", snap.Name, err)
	}
	if !waitForReady {
		return nil
	}

	return sna.WaitOnReadyToUse(ctx, snapCli, snap.Name, snap.Namespace)
}

func (sna *snapshotAlpha) WaitOnReadyToUse(ctx context.Context, snapCli snapshotclient.Interface, snapshotName, namespace string) error {
	return poll.Wait(ctx, func (context.Context) (bool, error) {
		snap, err := snapCli.VolumesnapshotV1alpha1().VolumeSnapshots(namespace).Get(snapshotName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// Error can be set while waiting for creation
		if snap.Status.Error != nil {
			return false, errors.New(snap.Status.Error.Message)
		}
		return (snap.Status.ReadyToUse && snap.Status.CreationTime != nil), nil
	})
}

func (sna *snapshotAlpha) getContent(ctx context.Context, snapCli snapshotclient.Interface, contentName string) (*snapshot.VolumeSnapshotContent, error) {
	return snapCli.VolumesnapshotV1alpha1().VolumeSnapshotContents().Get(contentName, metav1.GetOptions{})
}

func (sna *snapshotAlpha) getDeletionPolicyFromClass(snapCli snapshotclient.Interface, snapClassName string) (*snapshot.DeletionPolicy, error) {
	vsc, err := snapCli.VolumesnapshotV1alpha1().VolumeSnapshotClasses().Get(snapClassName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to find VolumeSnapshotClass: %s", snapClassName)
	}
	return vsc.DeletionPolicy, nil
}

func (sna *snapshotAlpha) Get(ctx context.Context, snapCli snapshotclient.Interface, name, namespace string) (*snapshot.VolumeSnapshot, error) {
	return snapCli.VolumesnapshotV1alpha1().VolumeSnapshots(namespace).Get(name, metav1.GetOptions{})
}

func (sna *snapshotAlpha) Delete(ctx context.Context, snapCli snapshotclient.Interface, name, namespace string) error {
	if err := snapCli.VolumesnapshotV1alpha1().VolumeSnapshots(namespace).Delete(name, &metav1.DeleteOptions{}); !apierrors.IsNotFound(err) {
		return err
	}
	// If the Snapshot does not exist, that's an acceptable error and we ignore it
	return nil
}

func (sna *snapshotAlpha) Clone(ctx context.Context, snapCli snapshotclient.Interface, name, namespace, cloneName, cloneNamespace string, waitForReady bool) error {
	snap, err := sna.Get(ctx, snapCli, name, namespace)
	if err != nil {
		return err
	}
	if !snap.Status.ReadyToUse {
		return errors.Errorf("Original snapshot is not ready, VolumeSnapshot: %s, Namespace: %s", cloneName, cloneNamespace)
	}
	if snap.Spec.SnapshotContentName == "" {
		return errors.Errorf("Original snapshot does not have content, VolumeSnapshot: %s, Namespace: %s", cloneName, cloneNamespace)
	}

	_, err = sna.Get(ctx, snapCli, cloneName, cloneNamespace)
	if err == nil {
		return errors.Errorf("Target snapshot already exists in target namespace, Volumesnapshot: %s, Namespace: %s", cloneName, cloneNamespace)
	}
	if !k8errors.IsNotFound(err) {
		return errors.Errorf("Failed to query target Volumesnapshot: %s, Namespace: %s: %v", cloneName, cloneNamespace, err)
	}

	src, err := sna.GetSource(ctx, snapCli, name, namespace)
	if err != nil {
		return errors.Errorf("Failed to get source")
	}
	return sna.CreateFromSource(ctx, snapCli, src, cloneName, cloneNamespace, waitForReady)
}
