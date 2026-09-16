# Releases: Git Tags & Docker Images

**Repo:** `https://github.com/BayCEER/bayeos-api.git` (branch `main`)

**CI:** `.github/workflows/docker-publish.yml` (the only workflow; no separate release workflow)

A "release" is driven entirely by a git tag; Docker image tags are derived from it automatically.

## When the workflow runs

```yaml
on:
  push:
    branches: [main]
    tags: ["v*"]
  workflow_dispatch:
```

- push to `main`
- push of any tag matching `v*` (e.g. `v0.2.0`)
- manual dispatch

## How Docker image tags are set

`docker/metadata-action` against `${{ vars.DOCKERHUB_REPOSITORY }}`:

```yaml
tags: |
  type=ref,event=branch
  type=raw,value=latest,enable={{is_default_branch}}
  type=semver,pattern={{version}}
  type=ref,event=tag
```

| Event           | Docker tags pushed                 |
| --------------- | ---------------------------------- |
| Push to `main`  | `:main`, `:latest`                 |
| Tag `v0.2.0`    | `:0.2.0` (semver), `:v0.2.0` (ref) |
| Manual dispatch | depends on branch/tag              |

## How the version is embedded

`Dockerfile`:

```dockerfile
ARG VERSION=dev
RUN go build -ldflags "-X main.Version=${VERSION}" -o bayeos-api .
```

Workflow passes `VERSION=${{ steps.meta.outputs.version || github.sha }}` → semver on tags, git SHA otherwise. Written into `main.Version` (`main.go:36`); logged at startup.

## Current state

- One tag exists: `v0.1.0`
- Recent `main` commits are after `v0.1.0` → only `:latest`/`:main` images, no new versioned tag
- No auto-generated GitHub Release (no release action)

## How to "release"

```bash
  git tag v0.2.0
git push origin v0.2.0
```

→ GitHub Actions builds and pushes `:0.2.0`, `:v0.2.0`, `:latest`, `:main` to Docker Hub.
