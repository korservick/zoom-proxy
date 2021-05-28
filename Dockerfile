#build stage
FROM golang:alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY . .
RUN go build -o /out/zoom-proxy

#final stage
FROM scratch AS bin
COPY --from=build /out/zoom-proxy /
ENTRYPOINT ./zoom-proxy
LABEL Name=zoom-proxy Version=0.0.1
EXPOSE 8080
ENV PORT=8080