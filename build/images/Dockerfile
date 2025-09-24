FROM alpine:3.20 AS builder
RUN apk add --no-cache build-base
WORKDIR /src
COPY init/init.c .
RUN gcc -static -Os -s -o overhyped-init init.c

FROM chromedp/headless-shell:stable
ARG OVERHYPED_AGENT_BINARY
COPY ${OVERHYPED_AGENT_BINARY} /usr/local/bin/hype-agent
COPY --from=builder /src/overhyped-init /init
RUN chmod +x /init /usr/local/bin/hype-agent

ENTRYPOINT ["/init"]
