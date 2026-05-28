FROM registry.access.redhat.com/ubi9/ubi-minimal:9.5

RUN microdnf install -y jq && microdnf clean all

COPY gangway-bridge.sh /usr/local/bin/gangway-bridge
RUN chmod +x /usr/local/bin/gangway-bridge

USER 1001
ENTRYPOINT ["/usr/local/bin/gangway-bridge"]
