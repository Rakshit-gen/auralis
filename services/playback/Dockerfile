# Build the playback service as a static binary and ship it on distroless.
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY libs/go-platform/go.mod libs/go-platform/go.sum ./libs/go-platform/
COPY services/playback/go.mod services/playback/go.sum ./services/playback/
RUN cd services/playback && go mod download

COPY libs/go-platform ./libs/go-platform
COPY services/playback ./services/playback
RUN cd services/playback && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/playback .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/playback /playback
EXPOSE 8084
USER nonroot:nonroot
ENTRYPOINT ["/playback"]
