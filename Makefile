GOCACHE?=.gocache

run:
	GOCACHE=$(GOCACHE) go run ./cmd/syncer init

test:
	GOCACHE=$(GOCACHE) go test ./...
