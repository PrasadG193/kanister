package snapshot

import (
	"context"

	snapshot "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	snapshotclient "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"

	"github.com/kanisterio/kanister/pkg/poll"
)

type snapshotBeta struct {
}

func (snb *snapshotBeta) GetVolumeSnapshotClass(annotation string, snapCli snapshotclient.Interface) (string, error) {
	vscl, err := snapCli.SnapshotV1beta1().VolumeSnapshotClasses().List(metav1.ListOptions{})
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

func (snb *snapshotBeta) Create(ctx context.Context, kubeCli kubernetes.Interface, snapCli snapshotclient.Interface, name, namespace, volumeName string, snapshotClass *string, waitForReady bool) error {
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
			Source: snapshot.VolumeSnapshotSource{
				PersistentVolumeClaimName: &volumeName,
			},
			VolumeSnapshotClassName: snapshotClass,
		},
	}

	_, err := snapCli.SnapshotV1beta1().VolumeSnapshots(namespace).Create(snap)
	if err != nil {
		return err
	}

	if !waitForReady {
		return nil
	}

	err = snb.WaitOnReadyToUse(ctx, snapCli, name, namespace)
	if err != nil {
		return err
	}

	_, err = snb.Get(ctx, snapCli, name, namespace)
	return err
}

func (snb *snapshotBeta) GetSource(ctx context.Context, snapCli snapshotclient.Interface, snapshotName, namespace string) (*Source, error) {
	snap, err := snb.Get(ctx, snapCli, snapshotName, namespace)
	if err != nil {
		return nil, errors.Errorf("Failed to get snapshot, VolumeSnapshot: %s, Error: %v", snapshotName, err)
	}
	cont, err := snb.getContent(ctx, snapCli, *snap.Status.BoundVolumeSnapshotContentName)
	if err != nil {
		return nil, errors.Errorf("Failed to get snapshot content, VolumeSnapshot: %s, VolumeSnapshotContent: %s, Error: %v", snapshotName, *snap.Status.BoundVolumeSnapshotContentName, err)
	}
	src := &Source{
		Handle:                  *cont.Status.SnapshotHandle,
		Driver:                  cont.Spec.Driver,
		RestoreSize:             cont.Status.RestoreSize,
		VolumeSnapshotClassName: cont.Spec.VolumeSnapshotClassName,
	}
	return src, nil
}

func (snb *snapshotBeta) CreateFromSource(ctx context.Context, snapCli snapshotclient.Interface, source *Source, snapshotName, namespace string, waitForReady bool) error {
	deletionPolicy, err := snb.getDeletionPolicyFromClass(snapCli, *source.VolumeSnapshotClassName)
	if err != nil {
		return errors.Wrap(err, "Failed to get DeletionPolicy from VolumeSnapshotClass")
	}
	contentName := snapshotName + "-content-" + string(uuid.NewUUID())
	content := &snapshot.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: snapshot.VolumeSnapshotContentSpec{
			Driver:         source.Driver,
			Source: snapshot.VolumeSnapshotContentSource{
				SnapshotHandle: &source.Handle,
			},
			VolumeSnapshotRef: corev1.ObjectReference{
				Kind:      snapshotKind,
				Namespace: namespace,
				Name:      snapshotName,
			},
			VolumeSnapshotClassName: source.VolumeSnapshotClassName,
			DeletionPolicy:          *deletionPolicy,
		},
	}
	snap := &snapshot.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: snapshotName,
		},
		Spec: snapshot.VolumeSnapshotSpec{
			Source: snapshot.VolumeSnapshotSource {
			VolumeSnapshotContentName: &content.Name,
			},
			VolumeSnapshotClassName: content.Spec.VolumeSnapshotClassName,
		},
	}

	content, err = snapCli.SnapshotV1beta1().VolumeSnapshotContents().Create(content)
	if err != nil {
		return errors.Errorf("Failed to create content, VolumesnapshotContent: %s, Error: %v", content.Name, err)
	}
	snap, err = snapCli.SnapshotV1beta1().VolumeSnapshots(namespace).Create(snap)
	if err != nil {
		return errors.Errorf("Failed to create content, Volumesnapshot: %s, Error: %v", snap.Name, err)
	}
	if !waitForReady {
		return nil
	}

	return snb.WaitOnReadyToUse(ctx, snapCli, snap.Name, snap.Namespace)
}

func (snb *snapshotBeta) WaitOnReadyToUse(ctx context.Context, snapCli snapshotclient.Interface, snapshotName, namespace string) error {
	return poll.Wait(ctx, func (context.Context) (bool, error) {
		snap, err := snapCli.SnapshotV1beta1().VolumeSnapshots(namespace).Get(snapshotName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// Error can be set while waiting for creation
		if snap.Status.Error != nil {
			return false, errors.New(*snap.Status.Error.Message)
		}
		return (snap.Status.ReadyToUse != nil && *snap.Status.ReadyToUse && snap.Status.CreationTime != nil), nil
	})
}

func (snb *snapshotBeta) getContent(ctx context.Context, snapCli snapshotclient.Interface, contentName string) (*snapshot.VolumeSnapshotContent, error) {
	return snapCli.SnapshotV1beta1().VolumeSnapshotContents().Get(contentName, metav1.GetOptions{})
}

func (snb *snapshotBeta) getDeletionPolicyFromClass(snapCli snapshotclient.Interface, snapClassName string) (*snapshot.DeletionPolicy, error) {
	vsc, err := snapCli.SnapshotV1beta1().VolumeSnapshotClasses().Get(snapClassName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to find VolumeSnapshotClass: %s", snapClassName)
	}
	return &vsc.DeletionPolicy, nil
}

func (snb *snapshotBeta) Get(ctx context.Context, snapCli snapshotclient.Interface, name, namespace string) (*snapshot.VolumeSnapshot, error) {
	return snapCli.SnapshotV1beta1().VolumeSnapshots(namespace).Get(name, metav1.GetOptions{})
}

func (snb *snapshotBeta) Delete(ctx context.Context, snapCli snapshotclient.Interface, name, namespace string) error {
	if err := snapCli.SnapshotV1beta1().VolumeSnapshots(namespace).Delete(name, &metav1.DeleteOptions{}); !apierrors.IsNotFound(err) {
		return err
	}
	// If the Snapshot does not exist, that's an acceptable error and we ignore it
	return nil
}

func (snb *snapshotBeta) Clone(ctx context.Context, snapCli snapshotclient.Interface, name, namespace, cloneName, cloneNamespace string, waitForReady bool) error {
	snap, err := snb.Get(ctx, snapCli, name, namespace)
	if err != nil {
		return err
	}
	if snap.Status.ReadyToUse == nil || !*snap.Status.ReadyToUse {
		return errors.Errorf("Original snapshot is not ready, VolumeSnapshot: %s, Namespace: %s", cloneName, cloneNamespace)
	}
	if snap.Spec.Source.VolumeSnapshotContentName == nil || *snap.Spec.Source.VolumeSnapshotContentName == "" {
		return errors.Errorf("Original snapshot does not have content, VolumeSnapshot: %s, Namespace: %s", cloneName, cloneNamespace)
	}

	_, err = snb.Get(ctx, snapCli, cloneName, cloneNamespace)
	if err == nil {
		return errors.Errorf("Target snapshot already exists in target namespace, Volumesnapshot: %s, Namespace: %s", cloneName, cloneNamespace)
	}
	if !k8errors.IsNotFound(err) {
		return errors.Errorf("Failed to query target Volumesnapshot: %s, Namespace: %s: %v", cloneName, cloneNamespace, err)
	}

	src, err := snb.GetSource(ctx, snapCli, name, namespace)
	if err != nil {
		return errors.Errorf("Failed to get source")
	}
	return snb.CreateFromSource(ctx, snapCli, src, cloneName, cloneNamespace, waitForReady)
}
