# Runtime base image. Parameterized so the same Dockerfile can build both the
# current UBI-minimal image (default) and a Red Hat Hardened Image (Hummingbird)
# variant via --build-arg RUNTIME_BASE=... . Declared before the first FROM so it
# is a global build arg usable in the runtime FROM below. The default MUST remain
# ubi-minimal so the production build is byte-for-byte unchanged.
ARG RUNTIME_BASE=registry.access.redhat.com/ubi9/ubi-minimal:9.8-1791898448

FROM quay.io/redhat-services-prod/openshift/boilerplate:latest AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# Copy the go source
COPY . .

# Build
# RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -mod vendor -o manager main.go
RUN make go-build

# Use a minimal/hardened base image to package the manager binary (see RUNTIME_BASE above).
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM ${RUNTIME_BASE}
WORKDIR /
COPY --from=builder /workspace/build/_output/bin/* /manager
USER nonroot:nonroot

ENTRYPOINT ["/manager"]
