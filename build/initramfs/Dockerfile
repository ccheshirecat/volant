FROM alpine:3.20 AS busybox
RUN apk add --no-cache busybox-static

FROM alpine:3.20
ARG VOLARY_BIN=volary
WORKDIR /initramfs

# staging tree
RUN mkdir -p root

# bring in binaries
COPY --from=busybox /bin/busybox root/bin/busybox
COPY ${VOLARY_BIN} root/bin/volary
COPY root/ root/

# make busybox usable before we call it
RUN chmod +x root/bin/busybox && \
    root/bin/busybox --install -s root/bin && \
    chmod +x root/init root/scripts/stage-volary.sh && \
    mkdir -p root/dev root/proc root/sys root/tmp root/run root/var/log root/etc &&  \
    touch root/etc/resolv.conf && \
    echo "nameserver 1.1.1.1" > root/etc/resolv.conf

CMD ["/bin/sh", "-c", "cd root && find . | cpio -H newc -o | gzip -9"]
