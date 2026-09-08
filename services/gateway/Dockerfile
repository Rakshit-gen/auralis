# Build the API gateway as a static binary and ship it on distroless.
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY libs/go-platform/go.mod libs/go-platform/go.sum ./libs/go-platform/
COPY services/gateway/go.mod services/gateway/go.sum ./services/gateway/
RUN cd services/gateway && go work init ../libs/go-platform . && go mod download

COPY libs/go-platform ./libs/go-platform
COPY services/gateway ./services/gateway
RUN cd services/gateway && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gateway"]
