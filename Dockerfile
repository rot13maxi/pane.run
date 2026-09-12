FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/surfaced ./cmd/surfaced

FROM scratch
COPY --from=build /out/surfaced /surfaced
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/surfaced", "-listen", ":8080", "-data", "/data/surfaces.json"]
