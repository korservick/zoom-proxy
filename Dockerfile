#build stage
FROM golang:alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /out/zoom-proxy

#final stage
FROM scratch AS bin
COPY --from=build /out/zoom-proxy /zoom-proxy
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/zoom-proxy"]
