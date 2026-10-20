CONTROLLER_GEN ?= $(shell go env GOPATH)/bin/controller-gen

$(CONTROLLER_GEN):
	cd hack && GOFLAGS="-mod=mod" go install sigs.k8s.io/controller-tools/cmd/controller-gen

.PHONY: build
build:
	GOFLAGS="-mod=mod" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o test-harness-operator ./cmd/test-harness-operator
	GOFLAGS="-mod=mod" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o test-harness-cluster-operator ./cmd/test-harness-cluster-operator
	GOFLAGS="-mod=mod" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o test-harness-ui ./cmd/test-harness-ui
	GOFLAGS="-mod=mod" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o reliability-runner ./cmd/reliability-runner

.PHONY: generate
generate: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) object paths="./pkg/api/..."

.PHONY: manifests
manifests: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) crd paths="./pkg/api/reliabilitytest/..." output:crd:artifacts:config=pkg/api/reliabilitytest/v1alpha1
	$(CONTROLLER_GEN) crd paths="./pkg/api/cluster/..." output:crd:artifacts:config=pkg/api/cluster/v1alpha1
	$(CONTROLLER_GEN) crd paths="./pkg/api/clusterprofiletemplate/..." output:crd:artifacts:config=pkg/api/clusterprofiletemplate/v1alpha1

.PHONY: test
test:
	GOFLAGS="-mod=readonly" GOMODCACHE=/tmp/gomodcache go test ./...
