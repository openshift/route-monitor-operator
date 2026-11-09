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
and `git diff --check` passes. Local verification uses `--platform linux/amd64`
on the arm64 workstation VM. Both container builds were attempted; production
was stopped to avoid duplicate compilation, and the hardened build lost its
connection during compilation (`unexpected EOF`). Build/run verification is
pending a serialized retry. External CI, scanning and cluster verification remain
separate from these local checks.
