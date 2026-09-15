smol:
	curl https://pico.sh/smol.css -o ./static/smol.css
.PHONY: smol

clean:
	rm -rf ./public
.PHONY: clean

img:
	docker build -t neurosnap/pgit:latest .
.PHONY: img

static:
	go run . \
		--out ./public \
		--label pgit \
		--clone-url "https://github.com/picosh/pgit.git" \
		--issues-url "https://github.com/picosh/pgit/issues" \
		--contrib-url "https://github.com/picosh/pgit/pulls" \
		--revs main \
		--max-commits 10
.PHONY: static

dev: static
	rsync -rv --delete ./public/ pgs.sh:/git-pgit-local/
.PHONY: dev
