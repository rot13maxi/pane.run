FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/paned ./cmd/paned

FROM scratch
COPY --from=build /out/paned /paned
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/paned", "-listen", ":8080", "-data", "/data/surfaces.json"]
