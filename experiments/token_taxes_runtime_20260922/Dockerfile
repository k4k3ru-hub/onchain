# Research-only overlay. Production MarketHub Dockerfile remains unchanged.
ARG BASE_IMAGE=k4k3ru-markethub-markethub:latest
FROM ${BASE_IMAGE}
ARG TARGETARCH
COPY linux-${TARGETARCH}/solc-linux-${TARGETARCH}-v0.8.34+commit.80d5c536 /opt/solc/0.8.34
COPY linux-${TARGETARCH}/solc-linux-${TARGETARCH}-v0.8.37+commit.f401782d /opt/solc/0.8.37
COPY supervise-${TARGETARCH} /opt/compiler-supervisor
USER 65532:65532
ENTRYPOINT ["/opt/compiler-supervisor"]
