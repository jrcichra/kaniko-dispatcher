FROM golang:1.17.7-alpine3.15 as builder
WORKDIR /kaniko
COPY . .
RUN CGO_ENABLED=0 go build -o kaniko-dispatcher

FROM scratch
COPY --from=builder /kaniko-dispatcher kaniko-dispatcher
CMD ["kaniko-dispatcher"]