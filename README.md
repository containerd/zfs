# [containerd](https://github.com/containerd/containerd) ZFS snapshotter plugin

[![Build Status](https://travis-ci.org/AkihiroSuda/containerd-zfs.svg)](https://travis-ci.org/AkihiroSuda/containerd-zfs)

ZFS snapshotter plugin for containerd.

## Install (as shared library)

The following command installs the plugin as `/var/lib/containerd/plugins/zfs-$GOOS-$GOARCH.so`.

```console
$ make
$ sudo make install
```

Note that the daemon binary needs to be exactly the version used for building the shared library.

Please refer to [`Makefile`](./Makefile) for the version known to work with.

## Install (static link to the daemon)

Put [`plugin.go`](plugin.go) to `$GOPATH/github.com/containerd/containerd/cmd/containerd/builtins_zfs.go`, and build the daemon manually:


## Usage

```console
$ zfs create -o mountpoint=/var/lib/containerd/io.containerd.snapshotter.v1.zfs your-zpool/containerd
```

Then update `/etc/containerd/config.toml` to use `io.containerd.snapshotter.v1.zfs` snapshotter.
