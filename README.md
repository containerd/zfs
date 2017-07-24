# [containerd](https://github.com/containerd/containerd) ZFS snapshotter plugin

[![Build Status](https://travis-ci.org/AkihiroSuda/containerd-zfs.svg)](https://travis-ci.org/AkihiroSuda/containerd-zfs)

ZFS snapshotter plugin for containerd.

This plugin is tested on Ubuntu, but should be easily portable to Solaris and FreeBSD as well when containerd supports them.

## Install (as shared library)

The following command installs the plugin as `/var/lib/containerd/plugins/zfs-$GOOS-$GOARCH.so`.

```console
$ make
$ sudo make install
```

Note that the daemon binary needs to be exactly the version used for building the shared library.

Please refer to [`Makefile`](./Makefile) for the version known to work with.

## Install (static link to the daemon)

Put [`plugin.go`](plugin.go) to `$GOPATH/src/github.com/containerd/containerd/cmd/containerd/builtins_zfs.go`, and build the daemon manually:


## Usage

1. Set up a ZFS filesystem.
```console
$ zfs create -o mountpoint=/var/lib/containerd/io.containerd.snapshotter.v1.zfs your-zpool/containerd
```

2. Start containerd.

3. e.g. `ctr pull --snapshotter=zfs ...`
