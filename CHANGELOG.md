# Changelog

Use spec: https://common-changelog.org/

# v2.0.0 - 2026-05-15

## Added

- `--issues-url` will add a button to nav to redirect users to bug tracking site
- `--contrib-url` will add a button to nav to redirect users to code contribution site
- Proper support for multiple parents in the commit viewer

## Changed

- Redesigned site
- Removed `--home-url` seems unnecessary, added complexity for little gain
- Removed `--desc` seems unnecessary, just use README
- When a revision in `--revs` doesn't exist, warn instead of exit with error
