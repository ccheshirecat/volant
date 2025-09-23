FROM alpine:3.20 AS builder
RUN apk add --no-cache build-base
WORKDIR /src
COPY init/init.c .
RUN gcc -static -Os -s -o viper-init init.c

FROM ghcr.io/chromedp/headless-shell:latest
ARG VIPER_AGENT_BINARY
COPY ${VIPER_AGENT_BINARY} /usr/local/bin/viper-agent
COPY --from=builder /src/viper-init /sbin/viper-init
RUN chmod +x /sbin/viper-init /usr/local/bin/viper-agent

ENTRYPOINT ["/sbin/viper-init"]
