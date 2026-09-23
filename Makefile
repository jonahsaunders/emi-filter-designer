# Convenience targets (the shell scripts do the same thing).
.PHONY: all run dev test fmt vet windows pages clean

all: test windows pages

run:
	go run .

dev:
	go run . -dev

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/EMIFilterDesigner.exe .

pages:
	sh ./build-pages.sh

clean:
	rm -rf dist pages
