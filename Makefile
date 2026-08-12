.PHONY: dev backend frontend test integration security verify build docker container-smoke release-archive

VERSION ?= 0.1.0

dev:
	go run ./cmd/planexus

backend:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/planexus ./cmd/planexus

frontend:
	cd web && npm ci && npm run build
	rm -rf internal/webui/dist
	cp -R web/dist internal/webui/dist

test:
	go test ./...
	cd web && npm test -- --run

integration:
	./scripts/integration-test.sh

security:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
	cd web && npm audit --audit-level=high

verify:
	go test -race ./...
	go vet ./...
	cd web && npm ci && npm run build && npm test -- --run && npm audit --audit-level=high
	./scripts/integration-test.sh

build: frontend backend

docker:
	docker build --build-arg VERSION=$(VERSION) -t planexus:v$(VERSION) .

container-smoke: docker
	./scripts/container-smoke-test.sh $(VERSION)

release-archive: docker
	mkdir -p dist
	docker save planexus:v$(VERSION) | gzip -9 > dist/planexus-v$(VERSION).tar.gz
	sha256sum dist/planexus-v$(VERSION).tar.gz > dist/planexus-v$(VERSION).tar.gz.sha256
