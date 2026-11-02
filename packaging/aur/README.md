# AUR package `doormutex-companion-bin`

Installs the companion, its systemd user unit and the Plasma widget on Arch Linux and derivatives (CachyOS, EndeavourOS, Manjaro) from the files of a GitHub release – nothing is compiled.

```bash
paru -S doormutex-companion-bin   # or: yay -S doormutex-companion-bin
```

## How it is updated

`PKGBUILD` here is a template (`pkgver=0.0.0`, checksums `SKIP`). On a release tag the workflow [`release.yml`](../../.github/workflows/release.yml) runs

```bash
packaging/aur/update.sh X.Y.Z checksums.txt   # sets pkgver, pkgrel=1, sha256sums, regenerates .SRCINFO
```

in an Arch container and pushes `PKGBUILD` + `.SRCINFO` to `ssh://aur@aur.archlinux.org/doormutex-companion-bin.git`. Without the secret `AUR_SSH_PRIVATE_KEY` the job is skipped.

## One-time setup (maintainer)

1. Create an account on <https://aur.archlinux.org/register> and add an SSH **public** key in *My Account*. Use a separate key for CI:

   ```bash
   ssh-keygen -t ed25519 -f ~/.ssh/aur-doormutex -C "doormutex AUR CI" -N ""
   ```

2. Create the package (the first push creates it):

   ```bash
   git clone ssh://aur@aur.archlinux.org/doormutex-companion-bin.git
   cd doormutex-companion-bin
   /path/to/doormutex/packaging/aur/update.sh X.Y.Z checksums.txt   # checksums.txt of the release
   cp /path/to/doormutex/packaging/aur/doormutex-companion-bin/{PKGBUILD,.SRCINFO} .
   makepkg -si   # test build + install
   git add PKGBUILD .SRCINFO && git commit -m "Initial release X.Y.Z" && git push
   ```

3. Store the **private** key as repository secret `AUR_SSH_PRIVATE_KEY` (*Settings → Secrets and variables → Actions*). From then on every release updates the AUR.

Don't commit the filled-in PKGBUILD back to this repository – the template stays at `0.0.0` (CI checks that `.SRCINFO` matches it).
