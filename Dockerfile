FROM spiksius/go-bash-1.19 AS build

RUN apk update && apk add ca-certificates
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /app

FROM scratch AS bin
COPY --from=build /app /app
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/app","--config", "/config.ini"]
