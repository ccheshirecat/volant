#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/sysmacros.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

// --- Utility Functions ---

static void print_error(const char *context) {
    fprintf(stderr, "volant Init ERROR in %s: %s\n", context, strerror(errno));
    fflush(stderr);
}

static void mount_fs(const char *src, const char *tgt, const char *type) {
    if (mkdir(tgt, 0755) && errno != EEXIST) {
        print_error(tgt);
    }
    if (mount(src, tgt, type, 0, "") != 0) {
        print_error(tgt);
    }
}

static void make_node(const char *path, mode_t mode, dev_t dev) {
    if (mknod(path, mode, dev) && errno != EEXIST) {
        print_error(path);
    }
}

// --- Daemonization and Process Management ---

// Spawns a command as a true daemon using the double-fork technique.
static void spawn_daemon(const char *path, char *const argv[]) {
    pid_t pid = fork();
    if (pid < 0) { print_error("fork1 for daemon"); return; }
    if (pid > 0) { waitpid(pid, NULL, 0); return; } // Parent waits for intermediate child

    // Intermediate child
    if (setsid() < 0) { _exit(1); } // Create a new session

    pid = fork();
    if (pid < 0) { _exit(1); }
    if (pid > 0) { _exit(0); } // Intermediate child exits

    // Grandchild (the daemon)
    umask(0);
    chdir("/");

    // Close all file descriptors
    for (int x = sysconf(_SC_OPEN_MAX); x >= 0; x--) {
        close(x);
    }
    
    // stdin, stdout, stderr to /dev/null
    open("/dev/null", O_RDWR);
    dup(0);
    dup(0);

    execv(path, argv);
    // This part should not be reached
    _exit(127);
}

// Spawns and supervises the main agent process.
static pid_t spawn_agent(void) {
    pid_t pid = fork();
    if (pid < 0) { print_error("fork for agent"); return -1; }
    if (pid == 0) { // Child process
        const char *agent_path = "/usr/local/bin/volary";
        char *const agent_argv[] = {(char *)agent_path, NULL};
        char *const agent_envp[] = {"PATH=/usr/local/bin:/usr/bin:/bin:/sbin", NULL};
        execve(agent_path, agent_argv, agent_envp);
        // This part is only reached if execve fails
        print_error("execve volary");
        _exit(1);
    }
    printf("[INIT] Launched volary with PID %d.\n", pid);
    fflush(stdout);
    return pid;
}

// --- Main Init Logic ---

int main(void) {
    // 1. Mount essential filesystems
    printf("[INIT] Mounting virtual filesystems...\n"); fflush(stdout);
    mount_fs("none", "/proc", "proc");
    mount_fs("none", "/sys", "sysfs");
    mount_fs("none", "/dev", "devtmpfs");
    mount_fs("none", "/run", "tmpfs");
    mount_fs("none", "/dev/pts", "devpts");
    mount_fs("none", "/dev/shm", "tmpfs");

    // 2. Create essential device nodes
    printf("[INIT] Creating device nodes...\n"); fflush(stdout);
    make_node("/dev/null", S_IFCHR | 0666, makedev(1, 3));
    make_node("/dev/zero", S_IFCHR | 0666, makedev(1, 5));
    make_node("/dev/random", S_IFCHR | 0444, makedev(1, 8));
    make_node("/dev/urandom", S_IFCHR | 0444, makedev(1, 9));
    make_node("/dev/tty", S_IFCHR | 0666, makedev(5, 0));
    make_node("/dev/console", S_IFCHR | 0622, makedev(5, 1));
    symlink("/proc/self/fd", "/dev/fd");

    // Redirect our own stdout/stderr to the console to see all logs
    int console_fd = open("/dev/console", O_WRONLY);
    if (console_fd >= 0) {
        dup2(console_fd, 1);
        dup2(console_fd, 2);
        if (console_fd > 2) close(console_fd);
    }

    // 3. Start D-Bus as a detached daemon
    printf("[INIT] Starting D-Bus daemon...\n"); fflush(stdout);
    char *const dbus_argv[] = {"/usr/bin/dbus-daemon", "--system", NULL};
    spawn_daemon("/usr/bin/dbus-daemon", dbus_argv);
    sleep(1); // Give D-Bus a moment to initialize

    // 4. Launch the interactive debug shell on the serial console
    printf("[INIT] Starting debug shell on /dev/ttyS0...\n"); fflush(stdout);
    if (fork() == 0) {
        setsid();
        int serial_fd = open("/dev/ttyS0", O_RDWR);
        if (serial_fd >= 0) {
            ioctl(serial_fd, TIOCSCTTY, 1);
            dup2(serial_fd, 0);
            dup2(serial_fd, 1);
            dup2(serial_fd, 2);
            if (serial_fd > 2) close(serial_fd);
            printf("\n--- Volant Debug Shell ---\n\n"); fflush(stdout);
            execl("/bin/sh", "sh", "-l", NULL);
        }
        _exit(1); // Exit if shell fails to start
    }

    // 5. Launch the main volary
    pid_t agent_pid = spawn_agent();

    // 6. Supervisor Loop: The heart of PID 1.
    printf("[INIT] Init complete. Now supervising volary.\n"); fflush(stdout);
    while (1) {
        int status;
        pid_t pid = wait(&status);
        if (pid > 0) {
            if (pid == agent_pid) {
                fprintf(stderr, "[INIT] CRITICAL: volary (PID %d) has exited. Restarting in 5s...\n", pid);
                fflush(stderr);
                sleep(5); // Prevent rapid crash loops
                agent_pid = spawn_agent();
            }
        }
    }

    return 0; // Unreachable
}