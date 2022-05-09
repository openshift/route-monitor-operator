# to build this, use the 'make packagemanifests-build' command

FROM quay.io/openshift/origin-operator-registry:4.10.0 AS builder

ARG BUNDLE_DIR=packagemanifests
COPY $BUNDLE_DIR manifests
RUN initializer --permissive

FROM registry.access.redhat.com/ubi8/ubi-micro:8.5-836

LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=route-monitor-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.2.0
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v2
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1

COPY --from=builder /bin/registry-server /bin/registry-server
COPY --from=builder /bin/grpc_health_probe /bin/grpc_health_probe

WORKDIR /registry
RUN chgrp -R 0 /registry && chmod -R g+rwx /registry

USER 1001

COPY --from=builder /registry /registry

EXPOSE 50051

CMD ["registry-server", "-t", "/tmp/terminate.log"]
