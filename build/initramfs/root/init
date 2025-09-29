#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/sysmacros.h>
#include <sys/wait.h>
#include <unistd.h>

static void log_line(const char *msg) {
    dprintf(STDOUT_FILENO, "[INIT] %s\n", msg);
}

static int run_command(char *const argv[]) {
    pid_t pid = fork();
    if (pid < 0) {
        return -1;
    }
    if (pid == 0) {
        execv(argv[0], argv);
        _exit(127);
    }
    int status = 0;
    if (waitpid(pid, &status, 0) < 0) {
        return -1;
    }
    if (WIFEXITED(status)) {
        return WEXITSTATUS(status);
    }
    return -1;
}

static void ensure_console(void) {
    struct stat st;
    if (stat("/dev/console", &st) != 0) {
        mknod("/dev/console", S_IFCHR | 0600, makedev(5, 1));
    }
    int fd = open("/dev/console", O_RDWR);
    if (fd >= 0) {
        dup2(fd, STDIN_FILENO);
        dup2(fd, STDOUT_FILENO);
        dup2(fd, STDERR_FILENO);
        if (fd > STDERR_FILENO) {
            close(fd);
        }
    }
}

static void mount_fs(const char *type, const char *target) {
    if (mount(type, target, type, 0, "") != 0 && errno != EBUSY) {
        char buf[256];
        snprintf(buf, sizeof(buf), "mount %s on %s failed: %s", type, target, strerror(errno));
        log_line(buf);
    }
}

static bool read_rootfs(char *buf, size_t buf_size) {
    FILE *f = fopen("/proc/cmdline", "r");
    if (!f) {
        return false;
    }
    char line[4096];
    if (!fgets(line, sizeof(line), f)) {
        fclose(f);
        return false;
    }
    fclose(f);

    const char *needle = "volant.rootfs=";
    char *start = strstr(line, needle);
    if (!start) {
        return false;
    }
    start += strlen(needle);
    char *end = strchr(start, ' ');
    size_t len = end ? (size_t)(end - start) : strlen(start);
    if (len >= buf_size) {
        len = buf_size - 1;
    }
    memcpy(buf, start, len);
    buf[len] = '\0';
    return true;
}

int main(void) {
    mount_fs("devtmpfs", "/dev");
    mount_fs("proc", "/proc");
    mount_fs("sysfs", "/sys");
    mount_fs("tmpfs", "/run");

    ensure_console();
    log_line("booting volant init");

    char rootfs[PATH_MAX];
    bool have_rootfs = read_rootfs(rootfs, sizeof(rootfs));
    const char *staging_script = "/scripts/stage-volary.sh";
    const char *fetch_script = "/scripts/fetch-rootfs.sh";

    if (have_rootfs) {
        log_line("rootfs specified; fetching image");
        mkdir("/sysroot", 0755);
        mkdir("/root", 0755);
        char *fetch_args[] = {(char *)fetch_script, rootfs, "/root/rootfs.img", NULL};
        if (access(fetch_script, X_OK) == 0 && run_command(fetch_args) == 0) {
            log_line("rootfs fetch complete; attempting mount");
            char *mount_args[] = {"/bin/mount", "-o", "loop", "/root/rootfs.img", "/sysroot", NULL};
            if (run_command(mount_args) == 0) {
                log_line("rootfs mounted; staging volary");
                if (access(staging_script, X_OK) == 0) {
                    char *stage_args[] = {(char *)staging_script, "/sysroot", NULL};
                    run_command(stage_args);
                }
                if (access("/sysroot/usr/local/bin/volary", X_OK) == 0) {
                    log_line("switching root to external rootfs");
                    execl("/bin/switch_root", "switch_root", "/sysroot", "/usr/local/bin/volary", NULL);
                    perror("switch_root");
                } else {
                    log_line("volary missing in mounted rootfs; continuing with initramfs");
                }
            } else {
                log_line("loop mount failed; continuing with initramfs");
            }
        } else {
            log_line("fetch-rootfs script failed; continuing with initramfs");
        }
    }

    if (access("/usr/local/bin/volary", X_OK) == 0) {
        log_line("launching volary from initramfs");
        execl("/usr/local/bin/volary", "volary", NULL);
        perror("exec volary");
    }

    if (access("/bin/volary", X_OK) == 0) {
        log_line("launching volary from /bin");
        execl("/bin/volary", "volary", NULL);
        perror("exec /bin/volary");
    }

    log_line("volary not found; dropping to rescue shell");
    execl("/bin/sh", "sh", NULL);
    return 1;
}
