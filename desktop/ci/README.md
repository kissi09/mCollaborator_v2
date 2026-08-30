# Parked CI workflow

`desktop-release.yml` is the GitHub Actions workflow that builds the Windows,
macOS and Linux desktop packages. It is the only place the last two can be built
at all — see `desktop/README.md` for why neither cross-compiles.

**It is parked here rather than in `.github/workflows/`, so it does not run yet.**

GitHub refuses a push from an OAuth app that creates or updates anything under
`.github/workflows/` unless the token carries the `workflow` scope:

```
! [remote rejected] refusing to allow an OAuth App to create or update
  workflow `.github/workflows/desktop-release.yml` without `workflow` scope
```

The token this repository pushes with has `gist`, `read:org` and `repo`, so the
rest of the branch could not go up while this file sat in the guarded directory.

## Activating it

Grant the scope once — it opens a browser for a one-time code and leaves every
other permission alone:

```bash
gh auth refresh -h github.com -s workflow
```

Then move the file into place and push:

```bash
git mv desktop/ci/desktop-release.yml .github/workflows/desktop-release.yml
git commit -m "Enable the desktop release workflow"
git push
```

Nothing else has to change. The workflow's contents are final; only its location
is waiting on the token.
