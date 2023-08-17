REV=$(shell git rev-parse --short HEAD)
PROJECT="git-pgit-$(REV)"

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
		--revs main
.PHONY:

deploy:
	scp -r ./public/* erock@pgs.sh:/$(PROJECT)
	ssh erock@pgs.sh git-pgit link $(PROJECT)
.PHONY: deploy
