FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/nash-load-api ./cmd/server

FROM alpine:3.20

RUN addgroup -S nash && adduser -S -G nash nash && mkdir -p /data && chown nash:nash /data
COPY --from=build /out/nash-load-api /usr/local/bin/nash-load-api

USER nash
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/nash-load-api"]
CMD ["-addr", ":8080", "-db", "/data/nash-load-lab.db"]
