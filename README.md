# pgit

A static site generator for git.

This golang binary will generate a commit log, files, and references based on a
git repository and the provided revisions.

It will only generate a commit log and files for the provided revisions.

# usage

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

# inspiration

This project was heavily inspired by
[stagit](https://codemadness.org/stagit.html)
