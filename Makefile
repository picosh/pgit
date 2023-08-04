clean:
	rm -rf ./public
.PHONY: clean

build:
	go build -o pgit ./main.go
.PHONY: build

static: build clean
	./pgit
.PHONY:

deploy:
	scp -R ./public/* erock@pgs.sh:/git
.PHONY: deploy
