FROM golang:1.20.1-bullseye as builder
WORKDIR /kaniko
COPY . .
RUN CGO_ENABLED=0 go build -o kaniko-dispatcher

FROM scratch
COPY --from=builder /kaniko/kaniko-dispatcher /kaniko-dispatcher
CMD ["/kaniko-dispatcher"]
