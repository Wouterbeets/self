.PHONY: test demo build

build:
	go build -o self .
	go build -o self-serve ./cmd/self-serve
	go build -o self-browse ./cmd/self-browse

test:
	go test ./...

demo:
	./demo.sh
