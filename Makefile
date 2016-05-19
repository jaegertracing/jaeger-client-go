PACKAGES := $(shell glide novendor)

export GO15VENDOREXPERIMENT=1


.PHONY: install
install:
	glide --version || go get github.com/Masterminds/glide
	glide install


.PHONY: test
test:
	go test -v $(PACKAGES)


.PHONY: cover
cover:
	./scripts/cover.sh $(shell go list $(PACKAGES))
	go tool cover -html=cover.out -o cover.html


# This is not part of the regular test target because we don't want to slow it
# down.
.PHONY: test-examples
test-examples:
	make -C examples


.PHONY: crossdock
crossdock:
	docker-compose kill go
	docker-compose rm -f go
	docker-compose build go
	docker-compose run crossdock


.PHONY: crossdock-fresh
crossdock-fresh: install
	docker-compose kill
	docker-compose rm --force
	docker-compose pull
	docker-compose build
	docker-compose run crossdock


.PHONY: install_ci
install_ci: install
	go get github.com/wadey/gocovmerge
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover


.PHONY: test_ci
test_ci:
	./scripts/cover.sh $(shell go list $(PACKAGES))
