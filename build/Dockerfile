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

# Runtime base image. Parameterized so the same Dockerfile can build both the
# current UBI-minimal image (default) and a Red Hat Hardened Image (Hummingbird)
# variant via --build-arg RUNTIME_BASE=... . The default MUST remain ubi-minimal
# so the production build is byte-for-byte unchanged.
# Refer to https://github.com/GoogleContainerTools/distroless for more details
ARG RUNTIME_BASE=registry.access.redhat.com/ubi9/ubi-minimal:9.8-1791898448
FROM ${RUNTIME_BASE}
WORKDIR /
COPY --from=builder /workspace/build/_output/bin/* /manager
USER nonroot:nonroot

ENTRYPOINT ["/manager"]
