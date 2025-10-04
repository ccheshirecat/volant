# Cloud-init Guide

Use Volant VMs as full-featured development environments with SSH access, user accounts, and automated configuration—just like cloud VMs from AWS, GCP, or DigitalOcean.

---

## What is Cloud-init?

[Cloud-init](https://cloud-init.io/) is the industry-standard multi-distribution method for cross-platform cloud instance initialization. It handles:

- ✅ User account creation
- ✅ SSH key injection
- ✅ Package installation
- ✅ File creation
- ✅ Command execution
- ✅ Network configuration
- ✅ And much more

**With Volant + cloud-init**, you can:

```bash
# Create a VM with your SSH key
volar vms create dev-env --plugin ubuntu --cloud-init user-data.yaml

# SSH in immediately
ssh ubuntu@192.168.127.100

# It's a full Linux environment
ubuntu@dev-env:~$ sudo apt install python3-pip
ubuntu@dev-env:~$ pip3 install flask
ubuntu@dev-env:~$ python3 app.py
```

---

## Quick Start

### Step 1: Create a cloud-init Configuration

Create `user-data.yaml`:

```yaml
#cloud-config
users:
  - name: developer
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC... your-key-here

packages:
  - git
  - curl
  - vim
  - build-essential

runcmd:
  - echo "Welcome to your Volant dev environment!" > /etc/motd
```

### Step 2: Create VM with Cloud-init

```bash
volar vms create dev-vm \
  --plugin ubuntu-cloud \
  --cloud-init user-data.yaml \
  --cpu 2 \
  --memory 2048
```

### Step 3: SSH In

```bash
# Wait for VM to boot (~5 seconds)
sleep 5

# SSH using the user you created
ssh developer@192.168.127.100

# Or find the IP automatically
IP=$(volar vms show dev-vm --json | jq -r '.ip')
ssh developer@$IP
```

**That's it!** You now have a fully configured development VM.

---

## Cloud-init Configuration Format

Cloud-init uses **YAML** with a special header: `#cloud-config`

```yaml
#cloud-config

# Everything below is standard cloud-init syntax
users:
  - name: myuser

packages:
  - package1
  - package2

runcmd:
  - echo "Hello World"
```

---

## Common Use Cases

### 1. SSH Key Injection

**The most common use case**: Add your SSH public key automatically.

```yaml
#cloud-config
users:
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC... your-public-key
```

**Get your public key**:

```bash
cat ~/.ssh/id_rsa.pub
```

**SSH into VM**:

```bash
ssh ubuntu@192.168.127.100
```

### 2. Multiple Users

```yaml
#cloud-config
users:
  - name: alice
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAAB3... alice-key

  - name: bob
    groups: docker
    ssh_authorized_keys:
      - ssh-rsa AAAAB3... bob-key

  - name: service-account
    system: true
    shell: /usr/sbin/nologin
```

### 3. Install Software

```yaml
#cloud-config
package_update: true
package_upgrade: true

packages:
  - docker.io
  - docker-compose
  - nodejs
  - npm
  - python3-pip
  - postgresql-client
```

### 4. Create Files

```yaml
#cloud-config
write_files:
  - path: /etc/myapp/config.yaml
    content: |
      database:
        host: db.example.com
        port: 5432
      logging:
        level: info
    owner: root:root
    permissions: '0644'

  - path: /home/ubuntu/app.py
    content: |
      from flask import Flask
      app = Flask(__name__)
      
      @app.route('/')
      def hello():
          return "Hello from Volant!"
      
      if __name__ == '__main__':
          app.run(host='0.0.0.0')
    owner: ubuntu:ubuntu
    permissions: '0755'
```

### 5. Run Commands on Boot

```yaml
#cloud-config
runcmd:
  # Clone your project
  - git clone https://github.com/youruser/yourapp /home/ubuntu/app
  - chown -R ubuntu:ubuntu /home/ubuntu/app

  # Install dependencies
  - cd /home/ubuntu/app && npm install

  # Start service
  - systemctl enable myapp
  - systemctl start myapp
```

### 6. Configure Hostname and Timezone

```yaml
#cloud-config
hostname: dev-server
fqdn: dev-server.local
timezone: America/New_York

# Update /etc/hosts
manage_etc_hosts: true
```

### 7. Docker Setup

```yaml
#cloud-config
users:
  - name: developer
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: docker
    ssh_authorized_keys:
      - ssh-rsa AAAAB3...

packages:
  - docker.io
  - docker-compose

runcmd:
  - systemctl enable docker
  - systemctl start docker
  - docker pull nginx
  - docker run -d -p 80:80 nginx
```

### 8. Development Environment

```yaml
#cloud-config
users:
  - name: dev
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/zsh
    ssh_authorized_keys:
      - ssh-rsa AAAAB3...

packages:
  - zsh
  - git
  - vim
  - tmux
  - curl
  - wget
  - jq
  - build-essential
  - python3
  - python3-pip
  - nodejs
  - npm

write_files:
  - path: /home/dev/.zshrc
    content: |
      # Oh My Zsh
      export ZSH="$HOME/.oh-my-zsh"
      ZSH_THEME="robbyrussell"
      plugins=(git docker kubectl)
      source $ZSH/oh-my-zsh.sh
    owner: dev:dev

runcmd:
  # Install Oh My Zsh
  - sudo -u dev sh -c "$(curl -fsSL https://raw.github.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended
  
  # Install development tools
  - pip3 install --upgrade pip
  - pip3 install ipython black flake8 pytest
  - npm install -g yarn typescript eslint prettier
```

---

## Plugin Support for Cloud-init

### Using Cloud Images

The easiest way is to use **cloud images** (pre-built with cloud-init support):

```toml
# fledge.toml
[plugin]
name = "ubuntu-cloud"
version = "22.04"
strategy = "oci_rootfs"

[oci_source]
image = "ubuntu:22.04"

[cloud_init]
enabled = true
datasources = ["NoCloud", "None"]
```

**Pre-built cloud images**:
- Ubuntu Cloud Images: `ubuntu:22.04`, `ubuntu:20.04`
- Debian: `debian:12`, `debian:11`
- Alpine: `alpine:3.18`
- Fedora: `fedora:38`

### Building Custom Cloud-init Plugins

Add cloud-init to your custom image:

```toml
# fledge.toml
[plugin]
name = "my-cloud-app"
strategy = "oci_rootfs"

[oci_source]
image = "ubuntu:22.04"

[cloud_init]
enabled = true

[[file_mappings]]
source = "./cloud-init-config"
dest = "/etc/cloud/cloud.cfg.d/99-volant.cfg"
```

---

## How Cloud-init Works in Volant

```
┌─────────────────────────────────────────────────────┐
│                   Host Machine                       │
│                                                      │
│  1. volar vms create --cloud-init user-data.yaml    │
│     ↓                                                │
│  2. volantd creates VM with cloud-init metadata     │
│     ↓                                                │
│  ┌──────────────────────────────────────┐           │
│  │         Cloud Hypervisor VM          │           │
│  │                                      │           │
│  │  3. Kernel boots → kestrel starts    │           │
│  │     ↓                                │           │
│  │  4. kestrel mounts cloud-init disk   │           │
│  │     (user-data + meta-data)          │           │
│  │     ↓                                │           │
│  │  5. cloud-init reads config          │           │
│  │     - Creates users                  │           │
│  │     - Installs packages              │           │
│  │     - Runs commands                  │           │
│  │     ↓                                │           │
│  │  6. VM ready for SSH                 │           │
│  └──────────────────────────────────────┘           │
└─────────────────────────────────────────────────────┘
```

**Technical details**:

1. **User-data encoding**: volantd encodes your `user-data.yaml` and creates a VFAT seed image (NoCloud datasource)
2. **Meta-data injection**: Adds VM metadata (instance-id, hostname, network config)
3. **Disk attachment**: Attaches seed image as virtual disk with label `CIDATA`
4. **Cloud-init execution**: Runs on first boot, configures system
5. **Persistent state**: cloud-init tracks what it's done in `/var/lib/cloud/`

---

## Cloud-init Datasources

Volant supports these datasources:

### NoCloud (Default)

Uses an ISO image with `user-data` and `meta-data` files.

```bash
# VM sees this structure:
/dev/sr0
├── user-data      # Your cloud-config
└── meta-data      # VM metadata (instance-id, hostname)
```

### ConfigDrive

Similar to NoCloud, uses a disk image:

```bash
volar vms create myvm --plugin ubuntu --cloud-init user-data.yaml --cloud-init-datasource configdrive
```

---

## Advanced Configuration

### Network Configuration

Configure static IP via cloud-init:

```yaml
#cloud-config
network:
  version: 2
  ethernets:
    eth0:
      addresses:
        - 192.168.127.150/24
      gateway4: 192.168.127.1
      nameservers:
        addresses:
          - 8.8.8.8
          - 8.8.4.4
```

**Note**: With Volant's IPAM, this might conflict. Use carefully.

### SSH Server Configuration

```yaml
#cloud-config
ssh_pwauth: false  # Disable password authentication
ssh_deletekeys: false  # Keep existing host keys

# Custom sshd_config
write_files:
  - path: /etc/ssh/sshd_config.d/custom.conf
    content: |
      PermitRootLogin no
      PasswordAuthentication no
      PubkeyAuthentication yes
      Port 22
```

### Swap File

```yaml
#cloud-config
swap:
  filename: /swapfile
  size: 2G
  maxsize: 2G
```

### Custom Bootcmd vs Runcmd

```yaml
#cloud-config

# bootcmd: Runs on every boot (before network)
bootcmd:
  - echo "This runs on every boot"

# runcmd: Runs once on first boot (after network)
runcmd:
  - echo "This runs once"
```

---

## Complete Example: Full Dev Environment

```yaml
#cloud-config

# Set hostname
hostname: volant-dev
fqdn: volant-dev.local

# Set timezone
timezone: UTC

# Users
users:
  - name: developer
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    groups: sudo, docker
    ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC... your-key

# Update and install packages
package_update: true
package_upgrade: true

packages:
  # Development tools
  - git
  - vim
  - tmux
  - curl
  - wget
  - jq
  - htop
  
  # Build essentials
  - build-essential
  - cmake
  
  # Languages
  - python3
  - python3-pip
  - python3-venv
  - nodejs
  - npm
  - golang-go
  
  # Containers
  - docker.io
  - docker-compose
  
  # Database clients
  - postgresql-client
  - redis-tools

# Create project directory
write_files:
  - path: /home/developer/README.md
    content: |
      # Development Environment
      
      Welcome to your Volant development VM!
      
      ## Installed:
      - Python 3 + pip
      - Node.js + npm
      - Go
      - Docker + Docker Compose
      - Git, vim, tmux
      
      ## Getting started:
      
      ```bash
      git clone https://github.com/youruser/yourproject
      cd yourproject
      ./setup.sh
      ```
    owner: developer:developer

  - path: /home/developer/.bashrc_custom
    content: |
      # Custom aliases
      alias ll='ls -alh'
      alias g='git'
      alias d='docker'
      alias dc='docker-compose'
      
      # Environment
      export EDITOR=vim
      export GOPATH=$HOME/go
      export PATH=$PATH:$GOPATH/bin
      
      # Prompt
      PS1='\[\033[01;32m\]\u@volant\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ '
    owner: developer:developer

# Run commands
runcmd:
  # Enable Docker
  - systemctl enable docker
  - systemctl start docker
  - usermod -aG docker developer
  
  # Python setup
  - pip3 install --upgrade pip
  - pip3 install ipython jupyter flask django pytest black flake8
  
  # Node.js setup
  - npm install -g yarn typescript ts-node eslint prettier nodemon
  
  # Go setup
  - sudo -u developer mkdir -p /home/developer/go/{bin,src,pkg}
  
  # Source custom bashrc
  - echo "source ~/.bashrc_custom" >> /home/developer/.bashrc
  - chown developer:developer /home/developer/.bashrc
  
  # Welcome message
  - echo "Development environment ready! SSH: ssh developer@$(hostname -I | awk '{print $1}')" > /etc/motd

# Final message
final_message: |
  Cloud-init finished!
  Development environment is ready.
  SSH: ssh developer@$HOSTNAME
  
  Happy coding!
```

**Use it**:

```bash
volar vms create dev --plugin ubuntu-cloud --cloud-init full-dev.yaml --cpu 4 --memory 4096

# Wait for boot
sleep 10

# SSH in
ssh developer@192.168.127.100

# Start developing!
developer@volant-dev:~$ python3 --version
Python 3.10.12

developer@volant-dev:~$ node --version
v18.16.0

developer@volant-dev:~$ docker --version
Docker version 24.0.5, build ced0996

developer@volant-dev:~$ go version
go version go1.20.3 linux/amd64
```

---

## Debugging Cloud-init

### Check Cloud-init Status

```bash
# Inside VM
cloud-init status

# Detailed output
cloud-init status --long

# Wait for completion
cloud-init status --wait
```

### View Logs

```bash
# Cloud-init logs
sudo cat /var/log/cloud-init.log
sudo cat /var/log/cloud-init-output.log

# From host
volar vms logs dev --file /var/log/cloud-init.log
```

### Re-run Cloud-init

```bash
# Clean state
sudo cloud-init clean

# Re-run
sudo cloud-init init
sudo cloud-init modules --mode config
sudo cloud-init modules --mode final
```

### Validate Configuration

```bash
# On host, before creating VM
cloud-init schema --config-file user-data.yaml
```

---

## Cloud-init + vsock

Cloud-init works with vsock networking too:

```bash
volar vms create secure-dev \
  --plugin ubuntu-cloud \
  --cloud-init user-data.yaml \
  --network vsock
```

**Access via proxy**:

```bash
# SSH through vsock proxy (future feature)
volar vms ssh secure-dev
```

---

## Tips and Best Practices

### 1. Use SSH Keys, Not Passwords

```yaml
#cloud-config
users:
  - name: ubuntu
    ssh_authorized_keys:
      - ssh-rsa AAAAB3...

# Disable password auth
ssh_pwauth: false
```

### 2. Pin Package Versions

```yaml
#cloud-config
packages:
  - python3=3.10.12-1
  - nodejs=18.16.0-1
```

### 3. Test Configurations Locally

```bash
# Validate syntax
cloud-init schema --config-file user-data.yaml

# Test in VM
volar vms create test --plugin ubuntu --cloud-init user-data.yaml
ssh ubuntu@$(volar vms show test --json | jq -r '.ip')
```

### 4. Use Templates for Teams

```yaml
#cloud-config
# team-dev-template.yaml

users:
  - name: ${USERNAME}
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ${SSH_KEY}

packages:
  - ${REQUIRED_PACKAGES}
```

**Use with env vars**:

```bash
export USERNAME=alice
export SSH_KEY="ssh-rsa AAAAB3..."
export REQUIRED_PACKAGES="git vim docker.io"

envsubst < team-dev-template.yaml > alice-user-data.yaml
volar vms create alice-dev --cloud-init alice-user-data.yaml
```

### 5. Document Your Cloud-init Configs

```yaml
#cloud-config
# Purpose: Development environment for backend team
# Includes: Python, PostgreSQL, Redis
# Maintained by: devops@company.com
# Last updated: 2025-10-04

users:
  - name: backend-dev
    # ... config
```

---

## Next Steps

- **[SSH Access Guide](3_ssh-access.md)** — Advanced SSH configuration
- **[Networking Guide](1_networking.md)** — Network modes for cloud-init
- **[Plugin Development](../4_plugin-development/1_introduction.md)** — Add cloud-init to custom plugins

---

**Official cloud-init documentation**: https://cloudinit.readthedocs.io/

---

*Turn microVMs into full development environments in seconds.*
