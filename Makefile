GO_BUILD=GOOS=linux go build -o bin/

all: build

build:
	$(GO_BUILD) ./cmd/...
clean:
	rm -rf bin/*
