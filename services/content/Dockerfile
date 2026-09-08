# Build the content service as a static binary and ship it on distroless.
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY libs/go-platform/go.mod libs/go-platform/go.sum ./libs/go-platform/
COPY services/content/go.mod services/content/go.sum ./services/content/
RUN cd services/content && go work init ../libs/go-platform . && go mod download

COPY libs/go-platform ./libs/go-platform
COPY services/content ./services/content
RUN cd services/content && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/content .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/content /content
EXPOSE 8082
USER nonroot:nonroot
ENTRYPOINT ["/content"]
