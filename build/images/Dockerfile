FROM alpine:3.20 AS builder
RUN apk add --no-cache build-base
WORKDIR /src
COPY init/init.c .
RUN gcc -static -Os -s -o volant-init init.c

FROM chromedp/headless-shell:stable
ARG VOLANT_AGENT_BINARY
COPY ${VOLANT_AGENT_BINARY} /usr/local/bin/volary
COPY --from=builder /src/volant-init /init
RUN chmod +x /init /usr/local/bin/volary

ENTRYPOINT ["/init"]
