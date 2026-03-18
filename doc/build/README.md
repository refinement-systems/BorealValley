# Build Images

## Pijul Package

The repository publishes a dedicated `borealvalley-pijul` image so the app images do not need to compile Rust and `pijul` during every build.

- Package image: `ghcr.io/refinement-systems/borealvalley-pijul:debian12`
- Exact package image: `ghcr.io/refinement-systems/borealvalley-pijul:1.0.0-beta.11-debian12`
- Registry cache: `ghcr.io/refinement-systems/borealvalley-cache:pijul`
- Dockerfile: [tools/deploy/Dockerfile.pijul](/Users/mjm/repo/BorealValley/tools/deploy/Dockerfile.pijul)

The package image contains:

- `/usr/local/bin/pijul`
- the shared libraries reported by `ldd`
- `/pijul-root/`, which mirrors the files to copy into downstream images

The current `pijul 1.0.0-beta.11` build also needs `libdbus-1-dev`. The package Dockerfile installs that in addition to the dependencies listed on the Pijul downloads page.

## Prerequisites

- Docker with `buildx`
- Login to GitHub Container Registry:

```bash
gh auth token | docker login ghcr.io -u refinement-systems --password-stdin
```

## Publish The Package

Publish the multi-platform image and its registry-backed build cache:

```bash
tools/deploy/docker-buildx.sh \
  --dockerfile tools/deploy/Dockerfile.pijul \
  --platforms linux/amd64,linux/arm64 \
  --tag ghcr.io/refinement-systems/borealvalley-pijul:debian12 \
  --tag ghcr.io/refinement-systems/borealvalley-pijul:1.0.0-beta.11-debian12 \
  --cache-ref ghcr.io/refinement-systems/borealvalley-cache:pijul \
  --push
```

To verify the pushed manifest:

```bash
docker buildx imagetools inspect ghcr.io/refinement-systems/borealvalley-pijul:debian12
```

To verify the package locally on an `arm64` machine without waiting for `amd64` emulation:

```bash
tools/deploy/docker-buildx.sh \
  --dockerfile tools/deploy/Dockerfile.pijul \
  --platforms linux/arm64 \
  --tag borealvalley-pijul:test \
  --cache-ref ghcr.io/refinement-systems/borealvalley-cache:pijul \
  --load

docker run --rm borealvalley-pijul:test --version
```

## Use The Package From App Builds

The app Dockerfiles now copy `pijul` from the published package image. The default source is `ghcr.io/refinement-systems/borealvalley-pijul:debian12`.

To override it for testing:

```bash
tools/deploy/docker-build.sh \
  --dockerfile tools/deploy/Dockerfile.prod \
  --build-arg PIJUL_IMAGE=borealvalley-pijul:test \
  --tag borealvalley-web:test
```

Or with `buildx`:

```bash
tools/deploy/docker-buildx.sh \
  --dockerfile tools/deploy/Dockerfile.prod \
  --build-arg PIJUL_IMAGE=ghcr.io/refinement-systems/borealvalley-pijul:debian12 \
  --tag ghcr.io/refinement-systems/borealvalley-web:latest \
  --push
```

## Cache Notes

`tools/deploy/Dockerfile.pijul` uses BuildKit cache mounts for:

- `apt` package metadata
- Cargo registry downloads
- Cargo git checkouts
- Cargo build outputs

This matters most when `linux/amd64` builds run under QEMU on an `arm64` host. The first cross-architecture build is still slow, but the registry cache dramatically reduces repeat work. If frequent `amd64` rebuilds are expected, prefer a native `amd64` buildx node over QEMU.
