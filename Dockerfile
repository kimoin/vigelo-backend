FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/vsrv ./cmd/vsrv

FROM alpine:3.20
RUN adduser -D -u 10001 vsrv
USER vsrv
COPY --from=build /out/vsrv /usr/local/bin/vsrv
EXPOSE 8090
ENTRYPOINT ["vsrv"]
