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

	"github.com/containerd/containerd/pkg/testutil"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/testsuite"
	"github.com/containerd/continuity/testutil/loopback"
	zfs "github.com/mistifyio/go-zfs"
	"github.com/pkg/errors"
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
