PROJECT_ROOT=github.com/uber/jaeger-client-go
PACKAGES := $(shell glide novendor | grep -v ./thrift/...)

export GO15VENDOREXPERIMENT=1

GOTEST=go test -v $(RACE)
GOLINT=golint
GOVET=go vet
GOFMT=go fmt
XDOCK_YAML=crossdock/docker-compose.yml

THRIFT_VER=0.9.3
THRIFT_IMG=thrift:$(THRIFT_VER)
THRIFT=docker run -v "${PWD}:/data" $(THRIFT_IMG) thrift
THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_GEN_DIR=thrift/gen

.PHONY: test
test:
	$(GOTEST) $(PACKAGES)


.PHONY: fmt
fmt:
	$(GOFMT) $(PACKAGES)


.PHONY: lint
lint:
	$(foreach pkg, $(PACKAGES), $(GOLINT) $(pkg) | grep -v crossdock/thrift || true;)
	$(GOVET) $(PACKAGES)


.PHONY: install
install:
	glide --version || go get github.com/Masterminds/glide
	glide install


.PHONY: cover
cover:
	./scripts/cover.sh $(shell go list $(PACKAGES))
	go tool cover -html=cover.out -o cover.html


# This is not part of the regular test target because we don't want to slow it
# down.
.PHONY: test-examples
test-examples:
	make -C examples


.PHONY: bins
bins:
	CGO_ENABLED=0 GOOS=linux time go build -a -installsuffix cgo -o crossdock/crossdock ./crossdock

# TODO at the moment we're not generating tchan_*.go files
thrift: idl-submodule thrift-image
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/agent.thrift
	sed -i '' 's|"zipkincore"|"$(PROJECT_ROOT)/thrift/gen/zipkincore"|g' $(THRIFT_GEN_DIR)/agent/*.go
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/sampling.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/idl/thrift/zipkincore.thrift
	rm -rf thrift/gen/*/*-remote

idl-submodule:
	git submodule init
	git submodule update

thrift-image:
	docker pull $(THRIFT_IMG)
	$(THRIFT) -version

.PHONY: crossdock
crossdock: bins
	docker-compose -f $(XDOCK_YAML) kill go
	docker-compose -f $(XDOCK_YAML) rm -f go
	docker-compose -f $(XDOCK_YAML) build go
	docker-compose -f $(XDOCK_YAML) run crossdock


.PHONY: crossdock-fresh
crossdock-fresh: bins
	docker-compose -f $(XDOCK_YAML) kill
	docker-compose -f $(XDOCK_YAML) rm --force
	docker-compose -f $(XDOCK_YAML) pull
	docker-compose -f $(XDOCK_YAML) build
	docker-compose -f $(XDOCK_YAML) run crossdock


.PHONY: install_ci
install_ci: install
	go get github.com/wadey/gocovmerge
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover


.PHONY: test_ci
test_ci:
	@./scripts/cover.sh $(shell go list $(PACKAGES))
