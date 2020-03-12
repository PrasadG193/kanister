// Copyright 2019 The Kanister Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package snapshot

import (
	"fmt"
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//snapshot "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	snapshot "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	snapshotclient "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/clientset/versioned"
	//snapshotclient "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8errors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/kanisterio/kanister/pkg/kube"
)

const (
	snapshotKind = "VolumeSnapshot"
	pvcKind      = "PersistentVolumeClaim"
	snapshotVersionAlpha = "v1alpha1"
	snapshotVersionBeta = "v1beta1"
	snapshotGroup = "snapshot.storage.k8s.io"
	volumeSnapClassesRes = "volumesnapshotclasses"
)

type Snapshotter interface {
	GetVolumeSnapshotClass(annotation string, snapCli snapshotclient.Interface) (string, error)
	// Create creates a VolumeSnapshot and returns it or any error happened meanwhile.
	//
	// 'name' is the name of the VolumeSnapshot.
	// 'namespace' is namespace of the PVC. VolumeSnapshot will be crated in the same namespace.
	// 'volumeName' is the name of the PVC of which we will take snapshot. It must be in the same namespace 'ns'.
	// 'waitForReady' will block the caller until the snapshot status is 'ReadyToUse'.
	// or 'ctx.Done()' is signalled. Otherwise it will return immediately after the snapshot is cut.
	Create(ctx context.Context, kubeCli kubernetes.Interface, snapCli snapshotclient.Interface, name, namespace, volumeName string, snapshotClass *string, waitForReady bool) error
	// Get will return the VolumeSnapshot in the namespace 'namespace' with given 'name'.
	//
	// 'name' is the name of the VolumeSnapshot that will be returned.
	// 'namespace' is the namespace of the VolumeSnapshot that will be returned.
	Get(ctx context.Context, snapCli snapshotclient.Interface, name, namespace string) (*snapshot.VolumeSnapshot, error)
	// Delete will delete the VolumeSnapshot and returns any error as a result.
	//
	// 'name' is the name of the VolumeSnapshot that will be deleted.
	// 'namespace' is the namespace of the VolumeSnapshot that will be deleted.
	Delete(ctx context.Context, snapCli snapshotclient.Interface, name, namespace string) error
	// Clone will clone the VolumeSnapshot to namespace 'cloneNamespace'.
	// Underlying VolumeSnapshotContent will be cloned with a different name.
	//
	// 'name' is the name of the VolumeSnapshot that will be cloned.
	// 'namespace' is the namespace of the VolumeSnapshot that will be cloned.
	// 'cloneName' is name of the clone.
	// 'cloneNamespace' is the namespace where the clone will be created.
	// 'waitForReady' will make the function blocks until the clone's status is ready to use.
	Clone(ctx context.Context, snapCli snapshotclient.Interface, name, namespace, cloneName, cloneNamespace string, waitForReady bool) error
	// GetSource will return the CSI source that backs the volume snapshot.
	//
	// 'snapshotName' is the name of the Volumesnapshot.
	// 'namespace' is the namespace of the Volumesnapshot.
	GetSource(ctx context.Context, snapCli snapshotclient.Interface, snapshotName, namespace string) (*Source, error)
	// CreateFromSource will create a 'Volumesnapshot' and 'VolumesnaphotContent' pair for the underlying snapshot source.
	//
	// 'source' contains information about CSI snapshot.
	// 'snapshotName' is the name of the snapshot that will be created.
	// 'namespace' is the namespace of the snapshot.
	// 'waitForReady' blocks the caller until snapshot is ready to use or context is cancelled.
	CreateFromSource(ctx context.Context, snapCli snapshotclient.Interface, source *Source, snapshotName, namespace string, waitForReady bool) error
	// WaitOnReadyToUse will block until the Volumesnapshot in namespace 'namespace' with name 'snapshotName'
	// has status 'ReadyToUse' or 'ctx.Done()' is signalled.
	WaitOnReadyToUse(ctx context.Context, snapCli snapshotclient.Interface, snapshotName, namespace string) error
}

// Source represents the CSI source of the Volumesnapshot.
type Source struct {
	Handle                  string
	Driver                  string
	RestoreSize             *int64
	VolumeSnapshotClassName *string
}

func NewSnapshotter() (Snapshotter, error) {
	dynCli, err := kube.NewDynamicClient()
	if err != nil {
		return nil, err
	}
	// Check if alpha API supported
	gvr := schema.GroupVersionResource{Group: snapshotGroup, Version: snapshotVersionAlpha, Resource: volumeSnapClassesRes}
	_, err = dynCli.Resource(gvr).Namespace("").List(metav1.ListOptions{})
	fmt.Printf("Error type isNotFound=%v\n", k8errors.IsNotFound(err))
	if err == nil || k8errors.IsNotFound(err) {
		//return &snapshotAlpha{}, nil
		return nil, fmt.Errorf("Not supported.")
	}
	// Check if alpha API supported
	gvr.Version = snapshotVersionBeta
	_, err = dynCli.Resource(gvr).Namespace("").List(metav1.ListOptions{})
	fmt.Printf("Error type isNotFound=%v\n", k8errors.IsNotFound(err))
	if err == nil {
		return &snapshotBeta{}, nil
	}
	return nil, fmt.Errorf("Not supported.")
}
