build:
	@go build -o bin/aichat

run: build
	@./bin/aichat
	
migrate:
	@migrate create -ext sql -dir store/migrations -seq $(name)

test:
	@go test -v ./...
	
.PHONY: build test run