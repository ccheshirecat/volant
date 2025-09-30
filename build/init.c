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
#include <dirent.h>

static void log_line(const char *msg) {
    dprintf(STDOUT_FILENO, "[INIT] %s\n", msg);
    FILE *f = fopen("/init.log", "a");
    if (f) {
        fprintf(f, "%s\n", msg);
        fclose(f);
    }
}

static void append_to_file(const char *path, const char *msg) {
    if (path == NULL || msg == NULL) {
        return;
    }
    FILE *f = fopen(path, "a");
    if (!f) {
        return;
    }
    fprintf(f, "%s\n", msg);
    fclose(f);
}

static void log_dev_entries(void) {
    DIR *dir = opendir("/dev");
    if (!dir) {
        log_line("opendir /dev failed");
        return;
    }
    struct dirent *entry;
    char buffer[256];
    int count = 0;
    while ((entry = readdir(dir)) != NULL) {
        if (entry->d_name[0] == '.') {
            continue;
        }
        snprintf(buffer, sizeof(buffer), "dev entry: %s", entry->d_name);
        log_line(buffer);
        if (++count >= 32) {
            break;
        }
    }
    closedir(dir);
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

    bool mounted_rootfs = false;
    if (have_rootfs) {
        log_line("rootfs specified; fetching image");
        mkdir("/sysroot", 0755);
        mkdir("/root", 0755);
        char *fetch_args[] = {(char *)fetch_script, rootfs, "/root/rootfs.img", NULL};
        if (access(fetch_script, X_OK) == 0 && run_command(fetch_args) == 0) {
            log_line("rootfs fetch complete; attempting loop mount");
            char *mount_args[] = {"/bin/mount", "-o", "loop", "/root/rootfs.img", "/sysroot", NULL};
            if (run_command(mount_args) == 0) {
                mounted_rootfs = true;
                mkdir("/sysroot/var", 0755);
                mkdir("/sysroot/var/log", 0755);
                append_to_file("/sysroot/var/log/volant-init.log", "mounted loop rootfs at /sysroot");
            } else {
                log_line("loop mount failed; will probe attached disks");
            }
        } else {
            log_line("fetch-rootfs script failed; will probe attached disks");
        }
    }

    log_dev_entries();
    if (!mounted_rootfs) {
        const char *candidates[] = {"/dev/vdb", "/dev/vda", "/dev/sdb", "/dev/sda", NULL};
        mkdir("/sysroot", 0755);
        for (int attempt = 0; attempt < 5 && !mounted_rootfs; attempt++) {
            for (int i = 0; candidates[i] != NULL && !mounted_rootfs; i++) {
                const char *device = candidates[i];
                if (access(device, F_OK) != 0) {
                    continue;
                }
                char msg[256];
                snprintf(msg, sizeof(msg), "attempting to mount %s (attempt %d)", device, attempt + 1);
                log_line(msg);
                char *mount_args[] = {"/bin/mount", "-t", "ext4", (char *)device, "/sysroot", NULL};
                if (run_command(mount_args) == 0) {
                    mounted_rootfs = true;
                    snprintf(msg, sizeof(msg), "mounted %s to /sysroot", device);
                    log_line(msg);
                    mkdir("/sysroot/var", 0755);
                    mkdir("/sysroot/var/log", 0755);
                    append_to_file("/sysroot/var/log/volant-init.log", msg);
                    break;
                }
                snprintf(msg, sizeof(msg), "mount %s failed", device);
                log_line(msg);
            }
            if (!mounted_rootfs) {
                sleep(1);
            }
        }
    }

    if (mounted_rootfs) {
        log_line("rootfs mounted; staging volary");
        append_to_file("/sysroot/var/log/volant-init.log", "rootfs mounted; preparing runtime mounts");
        mkdir("/sysroot/dev", 0755);
        mkdir("/sysroot/proc", 0555);
        mkdir("/sysroot/sys", 0555);
        char *mount_dev_args[] = {"/bin/mount", "-t", "devtmpfs", "devtmpfs", "/sysroot/dev", NULL};
        if (run_command(mount_dev_args) != 0) {
            append_to_file("/sysroot/var/log/volant-init.log", "mount devtmpfs failed");
        }
        char *mount_proc_args[] = {"/bin/mount", "-t", "proc", "proc", "/sysroot/proc", NULL};
        if (run_command(mount_proc_args) != 0) {
            append_to_file("/sysroot/var/log/volant-init.log", "mount proc failed");
        }
        char *mount_sys_args[] = {"/bin/mount", "-t", "sysfs", "sysfs", "/sysroot/sys", NULL};
        if (run_command(mount_sys_args) != 0) {
            append_to_file("/sysroot/var/log/volant-init.log", "mount sysfs failed");
        }
        append_to_file("/sysroot/var/log/volant-init.log", "runtime mounts ready; running stage-volary");
        if (access(staging_script, X_OK) == 0) {
            char *stage_args[] = {(char *)staging_script, "/sysroot", NULL};
            if (run_command(stage_args) != 0) {
                log_line("stage-volary script failed");
                append_to_file("/sysroot/var/log/volant-init.log", "stage-volary script failed");
            }
        }
        if (access("/sysroot/usr/local/bin/volary", X_OK) == 0) {
            log_line("switching root to external rootfs");
            append_to_file("/sysroot/var/log/volant-init.log", "switching root to external rootfs");
            execl("/bin/switch_root", "switch_root", "/sysroot", "/usr/local/bin/volary", NULL);
            perror("switch_root");
        } else {
            log_line("volary missing after staging; continuing with initramfs");
            append_to_file("/sysroot/var/log/volant-init.log", "volary missing after staging; continuing with initramfs");
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
