# Offline image release

## Naming contract

- Docker image: `planexus:v<version>`
- Release asset: `planexus-v<version>.tar.gz`

For version `0.1.0`, run:

```bash
make release-archive VERSION=0.1.0
```

This builds the React UI and static Go service into a non-root scratch image, then exports it to `dist/planexus-v0.1.0.tar.gz`. The archive contains only the Planexus service image. PostgreSQL remains an enterprise dependency supplied through `POSTGRES_DSN`.

Validate the artifact on a connected build host:

```bash
sha256sum -c dist/planexus-v0.1.0.tar.gz.sha256
gzip -t dist/planexus-v0.1.0.tar.gz
docker load < dist/planexus-v0.1.0.tar.gz
docker image inspect planexus:v0.1.0
```

Transfer the archive and its independently trusted checksum into the offline network, run `docker load`, and deploy with the four required environment variables.

## GitHub release

The `Offline image release` workflow runs for `v*` tags or manual semantic-version input. It builds `planexus:v<version>` and attaches only `planexus-v<version>.tar.gz` to the corresponding GitHub Release at `https://github.com/hkjang/Planexus`.

Create the GitHub release only after tests, vulnerability checks, container smoke test and archive load verification pass. Source code, environment files, database images and plaintext credentials must never be included in the release asset.
