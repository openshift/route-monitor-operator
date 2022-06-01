FROM quay.io/app-sre/boilerplate:image-v2.3.2 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY . .

# Build
RUN make go-build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/ubi-micro:8.5-836
WORKDIR /
COPY --from=builder /workspace/build/_output/bin/* .
USER nonroot:nonroot

ENTRYPOINT ["/route-monitor-operator"]
