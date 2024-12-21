# pgit

A static site generator for git.

This golang binary will generate a commit log, files, and references based on a
git repository and the provided revisions.

It will only generate a commit log and files for the provided revisions.

## usage

```bash
make build
```

```bash
./pgit --revs main --label pico --out ./public
```

To learn more about the options run:

```bash
./pgit --help
```

## themes

We support all [chroma](https://xyproto.github.io/splash/docs/all.html) themes.
We do our best to adapt the theme of the entire site to match the chroma syntax
highlighting theme. This is a "closet approximation" as we are not testing every
single theme.

```bash
./pgit --revs main --label pico --out ./public --theme onedark
```

The default theme is `dracula`. If you want to change the colors for your site,
we generate a `vars.css` file that you are welcome to overwrite before
deploying, it will _not_ change the syntax highlighting colors, only the main
site colors.

## with multiple repos

`--root-relative` sets the prefix for all links (default: `/`). This makes it so
you can run multiple repos and have them all live inside the same static site.

```bash
pgit \
  --out ./public/pico \
  --home-url "https://git.erock.io" \
  --revs main \
  --repo ~/pico \
  --root-relative "/pico/"

pgit \
  --out ./public/starfx \
  --home-url "https://git.erock.io" \
  --revs main \
  --repo ~/starfx \
  --root-relative "/starfx/"

echo '<html><body><a href="/pico">pico</a><a href="/starfx">starfx</a></body></html>' > ./public/index.html

rsync -rv ./public/ pgs.sh:/git
```

## inspiration

This project was heavily inspired by
[stagit](https://codemadness.org/stagit.html)
