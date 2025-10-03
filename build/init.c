// In build/init.c
#define _GNU_SOURCE
#include <unistd.h>
#include <errno.h>
#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <fcntl.h>
#include <sys/wait.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/reboot.h>
#include <sys/sysmacros.h>

// A proper shutdown function
__attribute__((noreturn)) static void poweroff(void) {
    fflush(stdout);
    fflush(stderr);
    reboot(RB_POWER_OFF);
    // If reboot fails, we're in big trouble.
    // Loop forever to prevent the kernel from panicking on our exit.
    for (;;) {
        sleep(3600);
    }
}

// A proper panic function for our init
static void panic(const char *what) {
    fprintf(stderr, "\n\nINIT PANIC: %s: %s\n\n", what, strerror(errno));
    poweroff();
}

// Ensure the console is available for logging
static void ensure_console(void) {
    // Create /dev/console if it doesn't exist (it should)
    if (mknod("/dev/console", S_IFCHR | 0600, makedev(5, 1)) && errno != EEXIST)
        panic("mknod(/dev/console)");

    int fd = open("/dev/console", O_RDWR);
    if (fd < 0)
        panic("open(/dev/console)");

    // Redirect stdin, stdout, and stderr to the console
    dup2(fd, 0);
    dup2(fd, 1);
    dup2(fd, 2);
    if (fd > 2)
        close(fd);
}

// A more robust filesystem setup
static void mount_filesystems(void) {
    // Mount the essentials for the Go runtime and other tools
    if (mount("proc", "/proc", "proc", 0, NULL))
        panic("mount(/proc)");
    if (mount("sysfs", "/sys", "sysfs", 0, NULL))
        panic("mount(/sys)");
    if (mount("devtmpfs", "/dev", "devtmpfs", 0, NULL))
        panic("mount(/dev)");

    // Create and mount tmpfs for runtime data
    mkdir("/tmp", 0777);
    if (mount("tmpfs", "/tmp", "tmpfs", 0, NULL))
        panic("mount(/tmp)");
    mkdir("/run", 0755);
    if (mount("tmpfs", "/run", "tmpfs", 0, NULL))
        panic("mount(/run)");
}

int main(int argc, char *argv[]) {
    // Create a basic directory structure first
    mkdir("/proc", 0755);
    mkdir("/sys", 0755);
    mkdir("/dev", 0755);
    mkdir("/bin", 0755); // For kestrel

    // Set up the essential filesystems
    mount_filesystems();

    // Now that /dev is mounted, ensure we have a console
    ensure_console();

    printf("C INIT: Basic environment is up. Handing off to kestrel...\n");

    // Hand over control to our real Go init program.
    // This will now become the new PID 1.
    char *const kestrel_argv[] = {"/bin/kestrel", NULL};
    execv("/bin/kestrel", kestrel_argv);

    // If execv returns, it failed. This is a catastrophe.
    panic("execv(/bin/kestrel)");

    return 1; // Unreachable
}
