.PHONY: run test

run:
	@(sleep 1; (command -v open >/dev/null && open http://127.0.0.1:7777) || (command -v xdg-open >/dev/null && xdg-open http://127.0.0.1:7777) || true) &
	go run .

test:
	go test ./...
