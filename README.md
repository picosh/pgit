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

# inspiration

This project was heavily inspired by
[stagit](https://codemadness.org/stagit.html)
