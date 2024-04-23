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

static: build clean
	./pgit \
		--out ./public \
		--label pgit \
		--desc "static site generator for git" \
		--clone-url "https://github.com/picosh/pgit.git" \
		--home-url "https://git.erock.io" \
		--theme "algol" \
		--revs main
.PHONY:

dev: static
	rsync -rv ./public/* pgs.sh:/git-pgit-local
.PHONY: dev
