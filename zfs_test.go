// +build linux

/*
   Copyright The containerd Authors.

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

package zfs

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "crypto/sha256"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/pkg/testutil"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/containerd/continuity/fs/fstest"
	"github.com/containerd/continuity/testutil/loopback"
	zfs "github.com/mistifyio/go-zfs"
	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
)

func newTestZpool() (string, func() error, error) {
	lo, err := loopback.New(1 << 30) // 1GiB
	if err != nil {
		return "", nil, err
	}
	zpoolName := fmt.Sprintf("testzpool-%d", time.Now().UnixNano())
	zpool, err := zfs.CreateZpool(zpoolName, nil, lo.File)
	if err != nil {
		return "", nil, err
	}
	return zpoolName, func() error {
		if err := zpool.Destroy(); err != nil {
			return err
		}
		return lo.Close()
	}, nil
}

func newSnapshotter() func(context.Context, string) (snapshots.Snapshotter, func() error, error) {
	return func(ctx context.Context, root string) (snapshots.Snapshotter, func() error, error) {
		testZpool, destroyTestZpool, err := newTestZpool()
		if err != nil {
			return nil, nil, err
		}
		testZFSMountpoint, err := ioutil.TempDir("", "containerd-zfs-test")
		if err != nil {
			return nil, nil, err
		}
		testZFSName := filepath.Join(testZpool, "containerd-zfs-test")
		testZFS, err := zfs.CreateFilesystem(testZFSName, map[string]string{
			"mountpoint": testZFSMountpoint,
		})
		if err != nil {
			return nil, nil, errors.Wrapf(err, "could not create zfs %q on %q", testZFSName, testZFSMountpoint)
		}
		snapshotter, err := NewSnapshotter(testZFSMountpoint)
		if err != nil {
			return nil, nil, err
		}

		return snapshotter, func() error {
			if err := snapshotter.Close(); err != nil {
				return err
			}
			if err := testZFS.Destroy(zfs.DestroyRecursive | zfs.DestroyRecursiveClones | zfs.DestroyForceUmount); err != nil {
				return err
			}
			if err := os.RemoveAll(testZFSMountpoint); err != nil {
				return err
			}
			return destroyTestZpool()
		}, nil
	}
}

func TestZFS(t *testing.T) {
	testutil.RequiresRoot(t)
	testsuite.SnapshotterSuite(t, "zfs", newSnapshotter())
}

// TestZFSUsage tests the zfs snapshotter's Usage implementation.
func TestZFSUsage(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory
	root, err := ioutil.TempDir("", "TestZFSUsage-")
	assert.NilError(t, err)
	defer os.RemoveAll(root)

	// Create the snapshotter
	z, closer, err := newSnapshotter()(ctx, root)
	assert.NilError(t, err)
	defer closer() //nolint:errcheck

	// Prepare empty base layer
	target := filepath.Join(root, "prepare-1")
	_, err = z.Prepare(ctx, target, "")
	assert.NilError(t, err)

	emptyLayerUsage, err := z.Usage(ctx, target)
	assert.NilError(t, err)

	// Check that the empty layer has non-zero size from metadata
	assert.Assert(t, emptyLayerUsage.Size > 0)

	err = z.Commit(ctx, filepath.Join(root, "layer-1"), target)
	assert.NilError(t, err)

	// Create a child layer with a 1MB file
	var (
		oneMB       int64 = 1048576 // 1MB
		baseApplier       = fstest.Apply(fstest.CreateRandomFile("/a", 12345679, oneMB, 0777))
	)

	target = filepath.Join(root, "prepare-2")
	mounts, err := z.Prepare(ctx, target, filepath.Join(root, "layer-1"))
	assert.NilError(t, err)

	err = mount.WithTempMount(ctx, mounts, baseApplier.Apply)
	assert.NilError(t, err)

	// Commit the second layer
	err = z.Commit(ctx, filepath.Join(root, "layer-2"), target)
	assert.NilError(t, err)

	layer2Usage, err := z.Usage(ctx, filepath.Join(root, "layer-2"))
	assert.NilError(t, err)

	// Should be at least 1 MB + fs metadata
	assert.Check(t, layer2Usage.Size > oneMB,
		"%d > %d", layer2Usage.Size, oneMB)

	// Create another child layer with a 2MB file
	var twoMB int64 = 2097152 // 2MB
	baseApplier = fstest.Apply(fstest.CreateRandomFile("/b", 12345679, twoMB, 0777))

	target = filepath.Join(root, "prepare-3")
	mounts, err = z.Prepare(ctx, target, filepath.Join(root, "layer-2"))
	assert.NilError(t, err)

	err = mount.WithTempMount(ctx, mounts, baseApplier.Apply)
	assert.NilError(t, err)

	err = z.Commit(ctx, filepath.Join(root, "layer-3"), target)
	assert.NilError(t, err)

	layer3Usage, err := z.Usage(ctx, filepath.Join(root, "layer-3"))
	assert.NilError(t, err)

	// Should be at least 2 MB + fs metadata
	assert.Check(t, layer3Usage.Size > twoMB,
		"%d > %d", layer3Usage.Size, twoMB)

	// Should not include the parent snapshot's usage
	assert.Check(t, layer3Usage.Size < (layer2Usage.Size+twoMB),
		"%d < %d", layer3Usage.Size, (layer2Usage.Size + twoMB))
}
