# Build the auth service as a static binary and ship it on distroless.
FROM golang:1.25-alpine AS build
WORKDIR /src

# The service module depends on the platform library by relative path, so both
# module trees are copied and wired with a build-scoped go.work.
COPY libs/go-platform/go.mod libs/go-platform/go.sum ./libs/go-platform/
COPY services/auth/go.mod services/auth/go.sum ./services/auth/
RUN cd services/auth && go mod download

COPY libs/go-platform ./libs/go-platform
COPY services/auth ./services/auth
RUN cd services/auth && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/auth .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/auth /auth
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/auth"]
