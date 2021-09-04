build:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags="-w -s" -o /bin/grace

lint:
	golangci-lint run

deps:
	go get -u ./...
	go mod tidy
	go mod vendor
