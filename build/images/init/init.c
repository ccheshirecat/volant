#define _GNU_SOURCE
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

static void fatal(const char *msg) {
    perror(msg);
    _exit(EXIT_FAILURE);
}

static void mount_fs(const char *source, const char *target, const char *type, unsigned long flags) {
    if (mount(source, target, type, flags, "") != 0) {
        fprintf(stderr, "mount %s on %s failed: %s\n", source, target, strerror(errno));
    }
}

static void run_cmd(const char *cmd) {
    int rc = system(cmd);
    if (rc != 0) {
        fprintf(stderr, "command failed (%d): %s\n", rc, cmd);
    }
}

static void configure_network(const char *ipcfg) {
    if (!ipcfg || ipcfg[0] == '\0') {
        fprintf(stderr, "missing ip= kernel parameter\n");
        return;
    }

    char buf[256];
    strncpy(buf, ipcfg, sizeof(buf) - 1);
    buf[sizeof(buf) - 1] = '\0';

    char *addr = strtok(buf, ":");
    char *gateway = strtok(NULL, ":");
    char *netmask = strtok(NULL, ":");
    char *hostname = strtok(NULL, ":");
    char *iface = strtok(NULL, ":");

    if (hostname && hostname[0] != '\0') {
        if (sethostname(hostname, strlen(hostname)) != 0) {
            fprintf(stderr, "sethostname failed: %s\n", strerror(errno));
        }
    }

    if (!iface || iface[0] == '\0') {
        iface = "eth0";
    }

    run_cmd("ip link set lo up");

    char cmd[256];
    snprintf(cmd, sizeof(cmd), "ip link set %s up", iface);
    run_cmd(cmd);

    if (addr && addr[0] != '\0') {
        if (netmask && netmask[0] != '\0') {
            snprintf(cmd, sizeof(cmd), "ip addr add %s/%s dev %s", addr, netmask, iface);
        } else {
            snprintf(cmd, sizeof(cmd), "ip addr add %s dev %s", addr, iface);
        }
        run_cmd(cmd);
    }
    if (gateway && gateway[0] != '\0') {
        snprintf(cmd, sizeof(cmd), "ip route add default via %s dev %s", gateway, iface);
        run_cmd(cmd);
    }
}

static const char *extract_ip_param(const char *cmdline) {
    const char *needle = "ip=";
    const char *p = strstr(cmdline, needle);
    if (!p) {
        return NULL;
    }
    p += strlen(needle);
    static char ipbuf[256];
    size_t i = 0;
    while (*p && *p != ' ' && i < sizeof(ipbuf) - 1) {
        ipbuf[i++] = *p++;
    }
    ipbuf[i] = '\0';
    return ipbuf;
}

int main(void) {
    mount_fs("proc", "/proc", "proc", 0);
    mount_fs("sysfs", "/sys", "sysfs", 0);
    mount_fs("devtmpfs", "/dev", "devtmpfs", 0);

    FILE *f = fopen("/proc/cmdline", "r");
    if (!f) fatal("open /proc/cmdline");
    char cmdline[1024];
    if (!fgets(cmdline, sizeof(cmdline), f)) {
        fclose(f);
        fatal("read /proc/cmdline");
    }
    fclose(f);

    const char *ipcfg = extract_ip_param(cmdline);
    configure_network(ipcfg);

    const char *agent = "/usr/local/bin/viper-agent";
    char *const argv[] = {"viper-agent", NULL};
    char *const envp[] = {NULL};
    execve(agent, argv, envp);
    fatal("execve viper-agent");
    return 0;
}
