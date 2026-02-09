build:
	@go build -o bin/aichat

run: build
	@./bin/aichat

test:
	@go test -v ./...
	
.PHONY: build test