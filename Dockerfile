# syntax=docker/dockerfile:1

# Der Build läuft immer auf der Plattform des Builders und cross-kompiliert
# für das Ziel — so braucht das arm64-Image keine langsame QEMU-Emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Statisches Binary, damit das Ergebnis auf einem leeren Image läuft.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/drop .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/drop /drop
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/drop"]
