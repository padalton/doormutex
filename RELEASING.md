# Releasing

Software releases come from a tag `vX.Y.Z` on `main`. Everything else is automatic except the upload to the KDE Store.

## Cut a release

```bash
git switch main && git pull
git tag -a v0.1.0 -m "doormutex 0.1.0"
git push origin v0.1.0
```

A tag with a suffix (`v0.2.0-rc1`) becomes a *pre-release* and is not pushed to the AUR.

## What CI does

| Workflow | Result |
|---|---|
| [`release.yml`](.github/workflows/release.yml) → GoReleaser ([`.goreleaser.yaml`](.goreleaser.yaml)) | GitHub release with changelog and:<br>• `doormutex-hub_X.Y.Z_linux_{amd64,arm64}.tar.gz` – binary, example config, compose file<br>• `doormutex-companion_X.Y.Z_linux_{amd64,arm64}.tar.gz` – binary, example config, user units for `~/.local/bin` and `/usr/bin`, the widget<br>• `doormutex-companion_X.Y.Z_linux_{amd64,arm64}.{deb,rpm,pkg.tar.zst}`<br>• `doormutex-X.Y.Z.plasmoid` – the widget with the version in `metadata.json`<br>• `checksums.txt` + `checksums.txt.sigstore.json` (keyless signature) |
| `release.yml` → AUR job | updates [`doormutex-companion-bin`](packaging/aur/) on the AUR – only if the repository secret `AUR_SSH_PRIVATE_KEY` exists, otherwise skipped with a notice |
| [`image.yml`](.github/workflows/image.yml) | container image `ghcr.io/padalton/doormutex-hub:X.Y.Z`, `:X.Y`, `:latest` |

CI tests the GoReleaser configuration on every pull request with a snapshot build (*Release · snapshot*). Locally:

```bash
go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=publish,sign
ls dist build
```

## KDE Store (manual)

The store has no upload API, so after each release:

1. Download `doormutex-X.Y.Z.plasmoid` from the release.
2. <https://store.kde.org> → the product page *doormutex* → *Edit* → *Files* → upload it, and set the version.

The first time: *Add Product* in the category *Plasma 6 Widgets*, licence Apache-2.0, link to this repository, a screenshot of the panel and the popup.

## AUR

The PKGBUILD in [`packaging/aur/doormutex-companion-bin/`](packaging/aur/doormutex-companion-bin/) is a template (`pkgver=0.0.0`); [`packaging/aur/update.sh`](packaging/aur/update.sh) fills in the version and checksums from `checksums.txt`. One-time setup and manual updates: [packaging/aur/README.md](packaging/aur/README.md).

## Verify a release

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/padalton/doormutex/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
```
