# build stage
FROM golang:1.24-alpine AS build-env
RUN apk add make git bash build-base protobuf-dev
WORKDIR /go/src/github.com/ohsu-comp-bio/funnel
COPY go.* .
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build make build

# final stage: use docker-in-docker image to support the local worker
FROM docker:cli
WORKDIR /opt/funnel
EXPOSE 8000 9090
ENV PATH="/app:$PATH"
COPY --from=build-env  /go/src/github.com/ohsu-comp-bio/funnel/funnel /app/
ENTRYPOINT ["/app/funnel"]
