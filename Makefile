# as of September 7, 2017 (v1.0.0-beta.0)
EXPECTED_CONTAINERD_COMMIT ?=d0457b221369fa22fc2facdf73266b67993b3959
CONTAINERD_COMMIT=$(shell test -d ../../../github.com/containerd/containerd && cd ../../../github.com/containerd/containerd && git rev-parse HEAD)

BINARIES=zfs.so zfs.test

Z="🇿"
ONI = "👹"

all: $(BINARIES)

zfs.so:
	@test -n "$(CONTAINERD_COMMIT)" || (echo "$(ONI) Please checkout github.com/containerd/containerd $(EXPECTED_CONTAINERD_COMMIT) under GOPATH."; false)
	@test "$(CONTAINERD_COMMIT)" = "$(EXPECTED_CONTAINERD_COMMIT)" || (echo "$(ONI) WARNING: expected github.com/containerd/containerd to be $(EXPECTED_CONTAINERD_COMMIT), got $(CONTAINERD_COMMIT)" )
	@echo "$(Z) Building $@ against containerd $(CONTAINERD_COMMIT)."
	go build -buildmode=plugin -o $@ plugin.go

echo-expected-containerd-commit:
	@echo $(EXPECTED_CONTAINERD_COMMIT)

zfs.test:
	go test -c ./snapshot/zfs

test: zfs.test
	./$< -test.v -test.root

clean:
	rm -f $(BINARIES)

install: zfs.so
	mkdir -p /var/lib/containerd/plugins
	cp -f $< /var/lib/containerd/plugins/zfs-$(shell go env GOOS)-$(shell go env GOARCH).so
	@echo "$(Z) The plugin has been installed as \"io.containerd.snapshotter.v1.zfs\"."
	@echo "$(Z) Note that the daemon needs to be exactly $(CONTAINERD_COMMIT)."

uninstall:
	rm -f /var/lib/containerd/plugins/zfs-$(shell go env GOOS)-$(shell go env GOARCH).so

.PHONY: echo-expected-containerd-commit test clean install uninstall
