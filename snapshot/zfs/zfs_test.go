// +build linux

package zfs

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/containerd/snapshot"
	"github.com/containerd/containerd/snapshot/testsuite"
	"github.com/containerd/containerd/testutil"
	zfs "github.com/mistifyio/go-zfs"
)

func newTestZpool(t *testing.T) (string, func()) {
	lo, destroyLo := testutil.NewLoopback(t, 1<<30) // 1GiB
	zpoolName := fmt.Sprintf("testzpool-%d", time.Now().UnixNano())
	zpool, err := zfs.CreateZpool(zpoolName, nil, lo)
	if err != nil {
		t.Fatal(err)
	}
	return zpoolName, func() {
		if err := zpool.Destroy(); err != nil {
			t.Fatal(err)
		}
		destroyLo()
	}
}

func newSnapshotter(t *testing.T) func(context.Context, string) (snapshot.Snapshotter, func(), error) {
	return func(ctx context.Context, root string) (snapshot.Snapshotter, func(), error) {
		testZpool, destroyTestZpool := newTestZpool(t)
		testZFSMountpoint, err := ioutil.TempDir("", "containerd-zfs-test")
		if err != nil {
			t.Fatal(err)
		}
		testZFSName := filepath.Join(testZpool, "containerd-zfs-test")
		testZFS, err := zfs.CreateFilesystem(testZFSName, map[string]string{
			"mountpoint": testZFSMountpoint,
		})
		if err != nil {
			t.Fatalf("could not create zfs %q on %q: %v", testZFSName, testZFSMountpoint, err)
		}
		snapshotter, err := NewSnapshotter(testZFSMountpoint)
		if err != nil {
			t.Fatal(err)
		}

		return snapshotter, func() {
			if err := testZFS.Destroy(zfs.DestroyRecursive | zfs.DestroyRecursiveClones | zfs.DestroyForceUmount); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(testZFSMountpoint); err != nil {
				t.Fatal(err)
			}
			destroyTestZpool()
		}, nil
	}
}

func TestZFS(t *testing.T) {
	testutil.RequiresRoot(t)
	testsuite.SnapshotterSuite(t, "zfs", newSnapshotter(t))
}
