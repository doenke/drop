# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Statisches Binary, damit das Ergebnis auf einem leeren Image läuft.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/drop .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/drop /drop
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/drop"]
