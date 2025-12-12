//go:build linux

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
	_ "crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/testsuite"
	"github.com/containerd/containerd/v2/pkg/testutil"
	"github.com/containerd/continuity/fs/fstest"
	"github.com/containerd/continuity/testutil/loopback"
	"github.com/mistifyio/go-zfs/v4"
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
		testZFSMountpoint, err := os.MkdirTemp("", "containerd-zfs-test")
		if err != nil {
			return nil, nil, err
		}
		testZFSName := filepath.Join(testZpool, "containerd-zfs-test")
		testZFS, err := zfs.CreateFilesystem(testZFSName, map[string]string{
			"mountpoint": testZFSMountpoint,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("could not create zfs %q on %q", testZFSName, testZFSMountpoint)
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
	root := t.TempDir()

	// Create the snapshotter
	z, closer, err := newSnapshotter()(ctx, root)
	if err != nil {
		t.Error(err)
	}
	defer func() { _ = closer() }()

	// Prepare empty base layer
	target := filepath.Join(root, "prepare-1")
	_, err = z.Prepare(ctx, target, "")
	if err != nil {
		t.Error(err)
	}

	emptyLayerUsage, err := z.Usage(ctx, target)
	if err != nil {
		t.Error(err)
	}

	// Check that the empty layer has non-zero size from metadata
	if emptyLayerUsage.Size <= 0 {
		t.Errorf("expected layer2Usage.Size to be > 0, got: %d", emptyLayerUsage.Size)
	}

	err = z.Commit(ctx, filepath.Join(root, "layer-1"), target)
	if err != nil {
		t.Error(err)
	}

	const (
		oneMB int64 = 1048576 // 1MB
		twoMB int64 = 2097152 // 2MB
	)

	// Create a child layer with a 1MB file
	baseApplier := fstest.Apply(fstest.CreateRandomFile("/a", 12345679, oneMB, 0o777))

	target = filepath.Join(root, "prepare-2")
	mounts, err := z.Prepare(ctx, target, filepath.Join(root, "layer-1"))
	if err != nil {
		t.Error(err)
	}

	err = mount.WithTempMount(ctx, mounts, baseApplier.Apply)
	if err != nil {
		t.Error(err)
	}

	// Commit the second layer
	err = z.Commit(ctx, filepath.Join(root, "layer-2"), target)
	if err != nil {
		t.Error(err)
	}

	layer2Usage, err := z.Usage(ctx, filepath.Join(root, "layer-2"))
	if err != nil {
		t.Error(err)
	}

	// Should be at least 1 MB + fs metadata
	if layer2Usage.Size <= oneMB {
		t.Errorf("expected layer2Usage.Size to be > %d, got: %d", oneMB, layer2Usage.Size)
	}

	// Create another child layer with a 2MB file
	baseApplier = fstest.Apply(fstest.CreateRandomFile("/b", 12345679, twoMB, 0o777))

	target = filepath.Join(root, "prepare-3")
	mounts, err = z.Prepare(ctx, target, filepath.Join(root, "layer-2"))
	if err != nil {
		t.Error(err)
	}

	err = mount.WithTempMount(ctx, mounts, baseApplier.Apply)
	if err != nil {
		t.Error(err)
	}

	err = z.Commit(ctx, filepath.Join(root, "layer-3"), target)
	if err != nil {
		t.Error(err)
	}

	layer3Usage, err := z.Usage(ctx, filepath.Join(root, "layer-3"))
	if err != nil {
		t.Error(err)
	}

	// Should be at least 2 MB + fs metadata
	if layer3Usage.Size <= twoMB {
		t.Errorf("expected layer3Usage.Size to be > %d, got: %d", twoMB, layer3Usage.Size)
	}

	// Should not include the parent snapshot's usage
	if layer3Usage.Size >= (layer2Usage.Size + twoMB) {
		t.Errorf("expected layer3Usage.Size to be < %d, got: %d", layer2Usage.Size+twoMB, layer3Usage.Size)
	}
}
