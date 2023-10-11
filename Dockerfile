FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH
COPY kaniko-dispatcher-$TARGETARCH /kaniko-dispatcher
CMD ["/kaniko-dispatcher"]
