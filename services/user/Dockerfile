# Build the user service as a static binary and ship it on distroless.
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY libs/go-platform/go.mod libs/go-platform/go.sum ./libs/go-platform/
COPY services/user/go.mod services/user/go.sum ./services/user/
RUN cd services/user && go mod download

COPY libs/go-platform ./libs/go-platform
COPY services/user ./services/user
RUN cd services/user && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/user .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/user /user
EXPOSE 8083
USER nonroot:nonroot
ENTRYPOINT ["/user"]
