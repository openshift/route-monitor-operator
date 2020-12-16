FROM registry.access.redhat.com/ubi8/ubi-minimal:latest AS kustomize-builder

RUN microdnf install -y golang make which
RUN microdnf install -y git

# install kustomize
RUN git clone https://github.com/kubernetes-sigs/kustomize.git
RUN cd kustomize && \
    cd kustomize && \
    go install .
RUN ~/go/bin/kustomize version

FROM quay.io/operator-framework/operator-sdk:latest

RUN rm -rf /.cache
COPY --from=kustomize-builder /root/go/bin/kustomize /usr/local/bin/kustomize

