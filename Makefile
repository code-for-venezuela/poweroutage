GO_BUILD=GOOS=linux go build -o bin/

all: build

build:
	$(GO_BUILD) ./cmd/poweroutage.go
clean:
	rm -rf bin/*
