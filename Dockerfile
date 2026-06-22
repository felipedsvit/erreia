# syntax=docker/dockerfile:1.7

FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/erreia ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/erreia /erreia
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/erreia"]
