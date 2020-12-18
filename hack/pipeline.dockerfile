FROM registry.access.redhat.com/ubi8/ubi-minimal:latest AS kustomize-builder

RUN microdnf install -y golang make which
RUN microdnf install -y git

# install kustomize
RUN git clone https://github.com/kubernetes-sigs/kustomize.git
RUN cd kustomize && \
      git checkout kustomize/v3.8.8 && \
    cd kustomize && \
    go install .
RUN ~/go/bin/kustomize version

FROM quay.io/operator-framework/operator-sdk:v1.2.0

# We need git to clone our repo
RUN microdnf install -y git
# Clean up after install
RUN rm -rf /.cache
# Copy kustomize binary from builder 
COPY --from=kustomize-builder /root/go/bin/kustomize /usr/local/bin/kustomize

# Set workdir so we have a known location to copy files from
RUN mkdir /pipeline
WORKDIR /pipeline
# Clone base repo into container
COPY bundler.sh .

# Make all the things
ENTRYPOINT ["./bundler.sh"]
