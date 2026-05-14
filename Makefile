REV=$(shell git rev-parse --short HEAD)
PROJECT="git-pgit-$(REV)"

smol:
	curl https://pico.sh/smol.css -o ./static/smol.css
.PHONY: smol

clean:
	rm -rf ./public
.PHONY: clean

build:
	go build -o pgit ./main.go
.PHONY: build

img:
	docker build -t neurosnap/pgit:latest .
.PHONY: img

fmt:
	go fmt ./...
.PHONY: fmt

lint:
	golangci-lint run
.PHONY: lint

test:
	go test ./...
.PHONY: test

static:
	go run main.go \
		--out ./public \
		--label pgit \
		--clone-url "https://github.com/picosh/pgit.git" \
		--issues-url "https://github.com/picosh/pgit/issues" \
		--contrib-url "https://github.com/picosh/pgit/pulls" \
		--revs main
.PHONY: static

dev: static
	rsync -rv --delete ./public/ pgs.sh:/git-pgit-local/
.PHONY: dev
