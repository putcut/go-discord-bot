FROM golang:1.21-bullseye as builder

WORKDIR /build
COPY . /build/

RUN go mod tidy
RUN go mod download
RUN CGO_ENABLED=0 go build -o ./main ./cmd/main.go

FROM gcr.io/distroless/static-debian11:nonroot

WORKDIR /

COPY --from=builder --chown=nonroot:nonroot /build/main /main
COPY --from=builder --chown=nonroot:nonroot /build/app_config.json /app_config.json

CMD ["/main"]