# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.25 AS build
WORKDIR /src

# TODO(build-context): go.mod has `replace github.com/foxcool/greedy-eye => ../greedy-eye`,
# which points outside this build context and breaks `docker build .` here.
# Fix options: (a) move the build context to ge/ and COPY both repos, or
# (b) drop the replace, tag+push the backend, and build with
# GOPRIVATE=github.com/foxcool/*. Left unfixed until the backend api/ move is tagged.
COPY go.* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8090
USER nonroot:nonroot
ENTRYPOINT ["/server"]
