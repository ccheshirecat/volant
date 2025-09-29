gcc -static init.c -o init
cd ..
make build
cd build
cp ../bin/volary .
chmod +x fetch-rootfs.sh stage-volary.sh
rm -rf /tmp/initramfs
mkdir -p /tmp/initramfs/{bin,dev,etc,home,mnt,proc,sys,usr/bin,sbin,usr/sbin,scripts}
cp init /tmp/initramfs
cp volary /tmp/initramfs/bin
cp fetch-rootfs.sh stage-volary.sh /tmp/initramfs/scripts
cd /tmp/initramfs
wget -P ./bin https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox
chmod +x ./bin/busybox
chroot . /bin/busybox --install -s
find . -print0 | cpio --null -ov --format=newc > initramfs.cpio 
gzip ./initramfs.cpio
