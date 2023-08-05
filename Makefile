clean:
	rm -rf ./public
.PHONY: clean

build:
	go build -o pgit ./main.go
.PHONY: build

static: build clean
	cp -R ./static ./public
	./pgit
.PHONY:

deploy:
	scp -r ./public/* erock@pgs.sh:/git
.PHONY: deploy
