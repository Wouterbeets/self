.PHONY: test demo build

build:
	go build -o self .

test:
	go test ./...

demo:
	./demo.sh
