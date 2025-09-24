#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
#include <sys/sysmacros.h>

/**
 * Prints a message to stderr and continues.
 */
static void print_error(const char *msg) {
    fprintf(stderr, "Viper Init ERROR: %s: %s\n", msg, strerror(errno));
}

/**
 * Mounts a filesystem, printing a non-fatal error on failure.
 */
static void mount_fs(const char *source, const char *target, const char *type, unsigned long flags) {
    if (mkdir(target, 0755) && errno != EEXIST) {
        print_error(target);
        return;
    }
    if (mount(source, target, type, flags, "") != 0) {
        print_error(target);
    }
}

/**
 * Creates a device node if it doesn't exist.
 */
static void make_node(const char *path, mode_t mode, dev_t dev) {
    if (mknod(path, mode, dev) && errno != EEXIST) {
        print_error(path);
    }
}

/**
 * Executes a shell command via fork/exec, which is safer than system().
 */
static void run_cmd(const char *command) {
    pid_t pid = fork();
    if (pid == -1) {
        print_error("fork for run_cmd");
        return;
    }
    if (pid == 0) { // Child process
        execl("/bin/sh", "sh", "-c", command, NULL);
        // If execl returns, it's an error
        print_error("execl in run_cmd");
        _exit(127);
    }
    // Parent process
    waitpid(pid, NULL, 0);
}

/**
 * Parses the kernel command line ip= parameter and configures the network.
 */
static void configure_network(const char *cmdline) {
    const char *needle = "ip=";
    const char *p = strstr(cmdline, needle);
    if (!p) {
        fprintf(stderr, "Viper Init Info: missing ip= param, skipping static network config.\n");
        run_cmd("ip link set eth0 up && udhcpc -i eth0 -t 5 -q");
        return;
    }
    p += strlen(needle);

    char ipcfg[256];
    size_t i = 0;
    while (*p && *p != ' ' && i < sizeof(ipcfg) - 1) {
        ipcfg[i++] = *p++;
    }
    ipcfg[i] = '\0';

    // ip=<addr>::<gateway>:<netmask>:<hostname>:<iface>
    char *addr = strtok(ipcfg, ":");
    strtok(NULL, ":"); // Skip empty part for gateway in some formats
    char *gateway = strtok(NULL, ":");
    char *netmask = strtok(NULL, ":");
    char *hostname = strtok(NULL, ":");
    char *iface = "eth0";

    if (hostname && hostname[0] != '\0') {
        if (sethostname(hostname, strlen(hostname)) != 0) {
            print_error("sethostname");
        }
    }

    char cmd[256];
    run_cmd("ip link set lo up");
    snprintf(cmd, sizeof(cmd), "ip link set %s up", iface);
    run_cmd(cmd);

    if (addr && netmask) {
        snprintf(cmd, sizeof(cmd), "ip addr add %s/%s dev %s", addr, netmask, iface);
        run_cmd(cmd);
    }
    if (gateway) {
        snprintf(cmd, sizeof(cmd), "ip route add default via %s dev %s", gateway, iface);
        run_cmd(cmd);
    }
    printf("[INIT] Network configured.\n");
}

int main(void) {
    // 1. Mount essential filesystems to create a minimal Linux environment
    printf("[INIT] Mounting virtual filesystems...\n");
    mount_fs("none", "/proc", "proc", 0);
    mount_fs("none", "/sys", "sysfs", 0);
    mount_fs("none", "/dev", "devtmpfs", 0);
    mount_fs("none", "/run", "tmpfs", 0);
    mkdir("/dev/pts", 0755);
    mount_fs("none", "/dev/pts", "devpts", 0);
    mkdir("/dev/shm", 0755);
    mount_fs("none", "/dev/shm", "tmpfs", 0);

    // 2. Create essential device nodes
    printf("[INIT] Creating device nodes...\n");
    make_node("/dev/null", S_IFCHR | 0666, makedev(1, 3));
    make_node("/dev/zero", S_IFCHR | 0666, makedev(1, 5));
    make_node("/dev/random", S_IFCHR | 0444, makedev(1, 8));
    make_node("/dev/urandom", S_IFCHR | 0444, makedev(1, 9));
    make_node("/dev/tty", S_IFCHR | 0666, makedev(5, 0));

    // 3. Configure network from kernel command line
    char cmdline[1024] = {0};
    int fd = open("/proc/cmdline", O_RDONLY);
    if (fd >= 0) {
        read(fd, cmdline, sizeof(cmdline) - 1);
        close(fd);
        configure_network(cmdline);
    }

    // 4. Start D-Bus system-wide instance (critical for Chromium)
    printf("[INIT] Starting D-Bus daemon...\n");
    if (fork() == 0) {
        execl("/usr/bin/dbus-daemon", "dbus-daemon", "--system", NULL);
        print_error("execl dbus-daemon");
        _exit(1);
    }

    // 5. Start a debug shell on the serial console
    if (fork() == 0) {
        setsid();
        int serial_fd = open("/dev/ttyS0", O_RDWR);
        if (serial_fd >= 0) {
            ioctl(serial_fd, TIOCSCTTY, 1);
            dup2(serial_fd, 0);
            dup2(serial_fd, 1);
            dup2(serial_fd, 2);
            if (serial_fd > 2) close(serial_fd);
            printf("\n[INIT] Serial debug shell is active.\n");
            execl("/bin/sh", "sh", "-l", NULL);
            print_error("execl shell");
        }
        _exit(1);
    }

    // 6. Launch the main viper-agent
    printf("[INIT] Launching viper-agent...\n");
    pid_t agent_pid = fork();
    if (agent_pid == 0) {
        const char *agent_path = "/usr/local/bin/viper-agent";
        char *const agent_argv[] = {(char *)agent_path, NULL};
        char *const agent_envp[] = {"PATH=/usr/local/bin:/usr/bin:/bin:/sbin", NULL};
        execve(agent_path, agent_argv, agent_envp);
        // This part is only reached if execve fails
        print_error("execve viper-agent");
        _exit(1);
    }

    // 7. Supervisor Loop: Act as the ultimate parent process.
    printf("[INIT] Init complete. Supervising child processes.\n");
    while (1) {
        int status;
        pid_t pid = wait(&status);
        if (pid > 0) {
            if (pid == agent_pid) {
                printf("[INIT] CRITICAL: viper-agent (PID %d) has exited with status %d. Restarting...\n", pid, status);
                sleep(2); // Prevent rapid crash loops
                agent_pid = fork();
                if (agent_pid == 0) {
                     const char *agent_path = "/usr/local/bin/viper-agent";
                     char *const agent_argv[] = {(char *)agent_path, NULL};
                     char *const agent_envp[] = {"PATH=/usr/local/bin:/usr/bin:/bin:/sbin", NULL};
                     execve(agent_path, agent_argv, agent_envp);
                     _exit(1);
                }
            } else {
                 printf("[INIT] Supervised process %d exited with status %d.\n", pid, status);
            }
        }
    }

    return 0; // Unreachable
}