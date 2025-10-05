  # Networking Guide
  
  Volant offers two networking modes: **bridge networking** (default) and **vsock** (maximum isolation). This guide covers both approaches, their trade-offs, and when to use each.
  
  ---
  
  ## Overview
  
  | Feature | Bridge Networking | vsock Networking |
  |---------|------------------|------------------|
  | **Isolation** | Network-level (private subnet) | Maximum (no network stack in guest) |
  | **Complexity** | Low (familiar Linux networking) | Medium (requires proxy setup) |
  | **Performance** | Near-native | Near-native |
  | **VM-to-VM** | Direct communication | Via host proxy only |
  | **Internet access** | Direct via NAT | Via kestrel proxy |
  | **Use case** | Default, development, most workloads | High-security, zero-trust environments |
  | **Debugging** | Standard tools (tcpdump, curl) | Limited (no network stack) |
  
  **TL;DR**: Start with **bridge networking** (it's the default). Switch to **vsock** when you need maximum isolation.
  
  ---
  
  ## Bridge Networking (Default)
  
  ### What It Is
  
  Bridge networking creates a private subnet (`192.168.127.0/24` by default) where all VMs communicate. A Linux bridge (`volant0`) connects VMs to the host, and iptables NAT provides internet access.
  
  ```
  ┌─────────────────────────────────────────────────────────┐
  │                      Host Machine                        │
  │                                                          │
  │  ┌────────────────────────────────────────┐             │
  │  │         volant0 (Bridge)                │             │
  │  │       192.168.127.1/24                  │             │
  │  └─┬────┬────┬────┬────┬───────────────────┘             │
  │    │    │    │    │    │                                 │
  │  ┌─▼──┐ ┌▼──┐ ┌▼──┐ ┌▼──┐ ┌▼───────────┐                │
  │  │tap0│ │tap1│ │tap2│ │tap3│ │   tapN   │                │
  │  └─┬──┘ └┬──┘ └┬──┘ └┬──┘ └┬───────────┘                │
  │    │     │     │     │     │                             │
  │  ┌─▼─────▼─────▼─────▼─────▼────────────┐                │
  │  │         iptables NAT                  │                │
  │  │   (192.168.127.0/24 → Internet)       │                │
  │  └────────────────┬───────────────────────┘               │
  │                   │                                       │
  │            ┌──────▼─────────┐                             │
  │            │   eth0 / wlan0 │ → Internet                  │
  │            └────────────────┘                             │
  │                                                          │
  │  ┌──────────┐  ┌──────────┐  ┌──────────┐               │
  │  │ VM .100  │  │ VM .101  │  │ VM .102  │               │
  │  │┌────────┐│  │┌────────┐│  │┌────────┐│               │
  │  ││virtio- ││  ││virtio- ││  ││virtio- ││               │
  │  ││net     ││  ││net     ││  ││net     ││               │
  │  │└────────┘│  │└────────┘│  │└────────┘│               │
  │  └──────────┘  └──────────┘  └──────────┘               │
  └─────────────────────────────────────────────────────────┘
  ```
  
  ### Setup
  
  Bridge networking is configured during initial setup:
  
  ```bash
  sudo volar setup
  
  # This creates:
  # - Bridge interface (volant0)
  # - IP address (192.168.127.1/24)
  # - iptables NAT rules
  # - IP forwarding
  # - Systemd service for persistence
  ```
  
  **Manual configuration** (if needed):
  
  ```bash
  # Create bridge
  sudo ip link add volant0 type bridge
  sudo ip addr add 192.168.127.1/24 dev volant0
  sudo ip link set volant0 up
  
  # Enable IP forwarding
  sudo sysctl -w net.ipv4.ip_forward=1
  
  # Configure NAT
  sudo iptables -t nat -A POSTROUTING -s 192.168.127.0/24 -j MASQUERADE
  sudo iptables -A FORWARD -i volant0 -j ACCEPT
  sudo iptables -A FORWARD -o volant0 -j ACCEPT
  ```
  
  ### How It Works
  
  1. **VM Creation**:
    - volantd allocates an IP from the pool (`.100` - `.254`)
    - Cloud Hypervisor creates a TAP device
    - TAP device is attached to the bridge
    - VM's virtio-net interface receives the IP
  
  2. **VM-to-Host**:
    ```bash
    # From inside VM
    curl http://192.168.127.1:8080/api/v1/health
    ```
  
  3. **VM-to-VM**:
    ```bash
    # From VM .100 to VM .101
    curl http://192.168.127.101:80/
    ```
  
  4. **VM-to-Internet**:
    ```bash
    # From inside VM
    curl https://google.com
    # Traffic flows: VM → bridge → NAT → eth0/wlan0 → Internet
    ```
  
  ### IP Address Management
  
  volantd maintains an IP lease table in SQLite:
  
  ```sql
  CREATE TABLE ip_leases (
    ip          TEXT PRIMARY KEY,
    vm_id       TEXT,
    leased_at   TIMESTAMP
  );
  ```
  
  **Allocation logic**:
  - IPs `.1` - `.99`: Reserved (host uses `.1`)
  - IPs `.100` - `.254`: Available for VMs
  - First-available allocation (deterministic)
  - Released automatically on VM deletion
  
  **Custom IP allocation** (future feature):
  
  ```bash
  volar vms create myvm --plugin nginx --ip 192.168.127.150
  ```
  
  ### DNS Configuration
  
  **Default**: VMs use public DNS resolvers.
  
  **Inside VM** (manually configure):
  
  ```bash
  # /etc/resolv.conf
  nameserver 8.8.8.8
  nameserver 8.8.4.4
  ```
  
  **Via cloud-init** (recommended):
  
  ```yaml
  #cloud-config
  write_files:
    - path: /etc/resolv.conf
      content: |
        nameserver 8.8.8.8
        nameserver 8.8.4.4
  ```
  
  **With dnsmasq** (optional host service):
  
  ```bash
  # Install dnsmasq on host
  sudo apt install dnsmasq
  
  # Configure for volant0
  sudo tee /etc/dnsmasq.d/volant.conf <<EOF
  interface=volant0
  dhcp-range=192.168.127.100,192.168.127.254,12h
  dhcp-option=6,8.8.8.8,8.8.4.4
  EOF
  
  sudo systemctl restart dnsmasq
  ```
  
  ### Port Forwarding to VMs
  
  Expose VM services to the public internet:
  
  ```bash
  # Forward host:8080 → VM 192.168.127.100:80
  sudo iptables -t nat -A PREROUTING -p tcp --dport 8080 -j DNAT --to-destination 192.168.127.100:80
  sudo iptables -A FORWARD -p tcp -d 192.168.127.100 --dport 80 -j ACCEPT
  ```
  
  **Persistent rules** (save after reboot):
  
  ```bash
  # Debian/Ubuntu
  sudo apt install iptables-persistent
  sudo netfilter-persistent save
  
  # RHEL/CentOS
  sudo iptables-save > /etc/sysconfig/iptables
  ```
  
  ### Debugging
  
  **Check bridge status**:
  
  ```bash
  ip addr show volant0
  bridge link show
  ```
  
  **List connected VMs**:
  
  ```bash
  bridge fdb show dev volant0
  ```
  
  **Capture traffic**:
  
  ```bash
  # On host (all bridge traffic)
  sudo tcpdump -i volant0 -n
  
  # Specific VM traffic
  sudo tcpdump -i vmtap-nginx-demo -n
  
  # From inside VM
  tcpdump -i eth0 -n
  ```
  
  **Test connectivity**:
  
  ```bash
  # Host → VM
  ping 192.168.127.100
  curl http://192.168.127.100:80/
  
  # VM → Host (from inside VM)
  ping 192.168.127.1
  
  # VM → Internet (from inside VM)
  ping 8.8.8.8
  curl https://example.com
  ```
  
  ---
  
  ## vsock Networking (Maximum Isolation)
  
  ### What It Is
  
  vsock (Virtual Socket) is a host-guest communication mechanism that **bypasses the network stack entirely**. VMs have no network interfaces, no IP addresses, and no direct internet access. All communication flows through a Unix domain socket on the host.
  
  ```
  ┌─────────────────────────────────────────────────────────┐
  │                      Host Machine                        │
  │                                                          │
  │  ┌────────────────────────────────────────┐             │
  │  │         volantd (Control Plane)        │             │
  │  │                                        │             │
  │  │  ┌──────────────────────────────────┐ │             │
  │  │  │   kestrel Proxy (Ingress)        │ │             │
  │  │  │   - HTTP/WebSocket forwarding    │ │             │
  │  │  │   - TLS termination              │ │             │
  │  │  │   - Authentication               │ │             │
  │  │  └────────┬─────────────────────────┘ │             │
  │  └───────────┼─────────────────────────────┘             │
  │              │                                           │
  │              │ vsock (context IDs)                       │
  │              │                                           │
  │     ┌────────┼────────┬─────────────┬──────────┐        │
  │     │        │        │             │          │        │
  │  ┌──▼────┐ ┌▼─────┐ ┌▼──────┐  ┌──▼──────┐          │
  │  │ VM 3  │ │ VM 4 │ │ VM 5  │  │ VM N    │          │
  │  │ (CID3)│ │(CID4)│ │(CID5) │  │(CID N)  │          │
  │  │       │ │      │ │       │  │         │          │
  │  │ NO    │ │ NO   │ │ NO    │  │ NO      │          │
  │  │ NETWORK│ │NETWORK│ │NETWORK │ │NETWORK │          │
  │  └───────┘ └──────┘ └───────┘  └─────────┘          │
  └─────────────────────────────────────────────────────────┘
  ```
  
  **Security benefits**:
  -  No network stack in guest (no TCP/IP attack surface)
  -  No IP addresses (VMs can't be scanned)
  -  No VM-to-VM communication (strict isolation)
  -  No direct internet access (all traffic proxied)
  -  Host controls all I/O (centralized policy enforcement)
  
  ### When to Use vsock
  
  **Perfect for**:
  -  **High-security workloads** — PCI-DSS, HIPAA, zero-trust
  -  **Financial services** — Payment processing, trading systems
  -  **Secrets management** — Key vaults, HSM proxies
  -  **Untrusted code** — Sandboxed AI models, user-submitted code
  -  **Compliance requirements** — Air-gapped environments
  
  **NOT ideal for**:
  - Development/testing (harder to debug)
  - Applications requiring VM-to-VM communication
  - Legacy apps expecting standard networking
  - Performance-critical workloads with high I/O
  
  ### How It Works
  
  1. **VM Creation with vsock**:
    ```bash
    volar vms create secure-app --plugin myapp --network vsock
    ```
  
  2. **Cloud Hypervisor configuration**:
    ```bash
    cloud-hypervisor \
      --vsock cid=3,socket=/var/run/volant/vsock-3.sock \
      # No --net flag (no network interface)
    ```
  
  3. **kestrel Proxy Setup**:
    - Listens on the vsock socket (host side)
    - Forwards HTTP/WebSocket requests to workload
    - Provides internet access for outbound requests (if configured)
  
  4. **Access Flow**:
    ```
    User → volantd → kestrel proxy → vsock → VM workload
    ```
  
  ### Configuration
  
  **Plugin manifest** (enable vsock support):
  
  ```json
  {
    "name": "secure-app",
    "version": "1.0.0",
    "networking": {
      "mode": "vsock",
      "proxy": {
        "enabled": true,
        "port": 8080
      }
    },
    "workload": {
      "entrypoint": ["/usr/bin/myapp"],
      "http": {
        "enabled": true,
        "port": 3000,
        "base_url": "http://localhost:3000"
      }
    }
  }
  ```
  
  **Create VM with vsock**:
  
  ```bash
  # Explicit vsock mode
  volar vms create secure-vm --plugin secure-app --network vsock
  
  # volantd constructs:
  # - vsock socket at /var/run/volant/vsock-<cid>.sock
  # - kestrel proxy listening on volantd (e.g., port 8080)
  # - No bridge/TAP device
  ```
  
  **Access the workload**:
  
  ```bash
  # Via volantd (kestrel proxies to VM)
  curl http://localhost:8080/api/v1/vms/secure-vm/proxy/
  ```
  
  ### kestrel as Proxy
  
  Inside the VM, kestrel acts as a reverse proxy:
  
  **Inside VM** (kestrel configuration):
  
  ```json
  {
    "proxy": {
      "listen": "vsock://0.0.0.0:8080",
      "upstream": "http://localhost:3000",
      "tls": {
        "enabled": false
      }
    }
  }
  ```
  
  **Host side** (volantd proxy):
  
  ```bash
  # volantd exposes VM via proxy endpoint
  # GET /api/v1/vms/secure-vm/proxy/* → vsock → kestrel → workload
  ```
  
  ### Outbound Internet Access (Optional)
  
  By default, vsock VMs have **no internet access**. Enable it explicitly:
  
  **Plugin manifest**:
  
  ```json
  {
    "networking": {
      "mode": "vsock",
      "proxy": {
        "enabled": true,
        "allow_outbound": true,
        "allowed_domains": [
          "api.example.com",
          "storage.googleapis.com"
        ]
      }
    }
  }
  ```
  
  **How it works**:
  1. Workload makes HTTP request (inside VM)
  2. Request goes to kestrel agent (inside VM)
  3. kestrel forwards via vsock to host
  4. volantd proxy makes request on behalf of VM
  5. Response flows back through vsock
  
  **Security**: Host enforces domain whitelist, TLS certificate validation, rate limiting.
  
  ### Debugging vsock
  
  **Check vsock sockets**:
  
  ```bash
  ls -la /var/run/volant/vsock-*.sock
  ```
  
  **Test vsock communication**:
  
  ```bash
  # From host
  socat - UNIX-CONNECT:/var/run/volant/vsock-3.sock
  # Type HTTP request manually
  GET / HTTP/1.1
  Host: localhost
  
  # From inside VM (if socat is available)
  socat - VSOCK-CONNECT:2:8080
  ```
  
  **Monitor traffic**:
  
  ```bash
  # volantd logs show proxy activity
  journalctl -u volantd -f | grep proxy
  
  # kestrel logs inside VM
  volar vms logs secure-vm --follow
  ```
  
  ---
  
  ## Networking Comparison
  
  ### Use Bridge When:
  
  -  You need VM-to-VM communication
  -  You want familiar networking tools
  -  You need to SSH directly to VMs
  -  You're developing/testing
  -  Private subnet isolation is sufficient
  -  You need to run standard network services
  
  ### Use vsock When:
  
  -  Maximum isolation is required
  -  Zero-trust security model
  -  Compliance mandates (PCI-DSS, HIPAA)
  -  Running untrusted code
  -  Need centralized traffic control
  -  Attack surface reduction is critical
  
  ---
  
  ## Custom Subnets
  
  Change the default bridge subnet:
  
  **Edit volantd configuration** (`/etc/volant/volantd.yaml`):
  
  ```yaml
  networking:
    bridge:
      name: volant0
      subnet: 10.200.0.0/16
      gateway: 10.200.0.1
      ip_range:
        start: 10.200.1.0
        end: 10.200.255.254
  ```
  
  **Restart volantd**:
  
  ```bash
  sudo systemctl restart volantd
  ```
  
  ---
  
  ## IPv6 Support (Future)
  
  Volant will support IPv6 bridge networking:
  
  ```yaml
  networking:
    bridge:
      ipv6:
        enabled: true
        subnet: fd00::/64
        gateway: fd00::1
  ```
  
  ---
  
  ## Advanced: Multiple Networks
  
  Run VMs on different isolated networks:
  
  ```bash
  # Create additional bridges
  sudo ip link add volant1 type bridge
  sudo ip addr add 192.168.128.1/24 dev volant1
  sudo ip link set volant1 up
  
  # Create VM on specific bridge
  volar vms create vm1 --plugin nginx --network bridge=volant1
  ```
  
  ---
  
  ## Network Policies (Future)
  
  Declarative network policies for traffic control:
  
  ```yaml
  # /etc/volant/network-policies.yaml
  policies:
    - name: allow-http
      vms: ["web-*"]
      rules:
        - protocol: tcp
          port: 80
          action: allow
  
    - name: deny-all
      vms: ["*"]
      rules:
        - action: deny
  ```
  
  ---
  
  ## Troubleshooting
  
  ### Bridge networking issues
  
  **Problem**: VM can't reach internet
  
  **Solutions**:
  ```bash
  # Check IP forwarding
  cat /proc/sys/net/ipv4/ip_forward  # Should be 1
  
  # Check NAT rules
  sudo iptables -t nat -L -n -v
  
  # Check bridge
  ip addr show volant0
  ```
  
  **Problem**: VM can't get IP
  
  **Solutions**:
  ```bash
  # Check IPAM
  volar debug ipam
  
  # Manually release IP
  volar vms delete <vm> --force
  ```
  
  ### vsock issues
  
  **Problem**: Can't connect via proxy
  
  **Solutions**:
  ```bash
  # Check socket exists
  ls -la /var/run/volant/vsock-*.sock
  
  # Check kestrel is running inside VM
  volar vms logs <vm>
  
  # Test vsock directly
  socat - UNIX-CONNECT:/var/run/volant/vsock-3.sock
  ```
  
  **Problem**: Outbound requests fail
  
  **Solution**: Ensure `allow_outbound: true` in manifest
  
  ---
  
  ## Next Steps
  
  - **[Cloud-init Guide](2_cloud-init.md)** — Configure networking via cloud-init
  - **[Scaling Guide](3_scaling.md)** — Network considerations for deployments
  - **[Security Model](../5_architecture/4_security.md)** — Network isolation deep-dive
  
  ---
  
  *Choose the right networking mode for your security posture.*
