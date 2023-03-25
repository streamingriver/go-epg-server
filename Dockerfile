FROM golang:buster AS build

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /app

FROM scratch AS bin
COPY --from=build /app /app

ENTRYPOINT ["/app","-config", "/config.ini"]
