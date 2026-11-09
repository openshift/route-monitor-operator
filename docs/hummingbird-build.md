# Hummingbird parallel build (ROSAENG-75340)

The production `build/Dockerfile` is unchanged. The separate hardened Dockerfile
retains the boilerplate compilation stage and packages `/manager` on the
FIPS-capable `hi/core-runtime:2.43-openssl-fips` runtime, pinned to a digest resolved
with `skopeo inspect --override-os linux --override-arch amd64`.

No package installation or external runtime command was found in the application
sources. The Go toolchain is retained because `make go-build` enables FIPS and
external linking (`CGO_ENABLED=1`, `GOFIPS140=v1.0.0`, `-linkmode=external`). No
Hummingbird builder stage, RPM, tarball dependency or UBI runtime filesystem copy
is needed. The linked binary still requires empirical runtime verification on
the new base. Numeric UID/GID 65532 avoids depending on a named account in the
runtime image.

The copied non-PKO push PipelineRun changes only its name, application/component
labels, output tag and Dockerfile parameter. Distinct labels and a separate tag
keep the image outside production snapshots. No deployment or release change is
included. Existing pilot PR #643 uses a base-image ARG; this implementation keeps
concrete FROM references for Renovate instead and does not apply that pilot.

JSON and YAML parse, the copied PipelineRun differs only in the requested fields,
and `git diff --check` passes. The production Dockerfile remains byte-for-byte
identical to the checkout.

## Local verification

Both Dockerfiles built successfully on the arm64 workstation's Podman VM using
Linux amd64 emulation. The final images were:

- hardened: `59978ac74551398b19f7ffd0409993912c9406e6ddb492346999f649d50d6bab`
- production: `03f7d70d70ded04f51ee077ab4fca4c12afe62bbf7ac263b329ab695dfdb0daf`

```sh
podman build --platform linux/amd64 --cpu-quota=100000 \
  --volume /tmp/hummingbird-go-cache:/tmp/go-build:z --pull=never \
  -f build/Dockerfile.hummingbird -t route-monitor-operator-hummingbird-test .
podman build --platform linux/amd64 --cpu-quota=300000 \
  --volume /var/tmp/hummingbird-go-cache:/tmp/go-build:z --pull=never \
  -f build/Dockerfile -t route-monitor-operator-production-test .
```

The shared cache is local verification infrastructure, not a Dockerfile or CI
requirement. The hardened compile's live CPU quota was raised to three CPUs; the
cache was subsequently moved from tmpfs to disk before the production build.
Earlier parallel attempts disconnected (`unexpected EOF`); the completed builds
above replaced those attempts.

`podman run --rm --platform linux/amd64 --entrypoint /manager
route-monitor-operator-hummingbird-test --help` passed, as did the same command
with `--user 1001230000:0`. The generated FIPS startup assertion ran successfully
before flag processing. `file` reports the executable as dynamically linked with
interpreter `/lib64/ld-linux-x86-64.so.2`; its successful runs demonstrate the
Hummingbird runtime supplies the necessary loader and startup libraries.

The unchanged production image builds, but its default `USER nonroot:nonroot`
fails before execution: `unable to find user nonroot: no matching entries in
passwd file` (exit 126). Running the production `/manager --help` with explicit
`--user 65532:65532` passes. This existing production configuration issue is
reported without modifying the production Dockerfile.

A live OpenShift cluster, full reconciliation, external CI, scanning and release
gates were not exercised by these local build and startup checks.
