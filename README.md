# Axion - VPS Control Tool

A Go CLI tool to control multiple VPS instances via SSH using a YAML configuration file.

## Installation

**Using Go:**
```
go install github.com/mrmahile/axion@latest
```

**Pre-built Binaries:**
```
wget https://github.com/mrmahile/axion/releases/download/v0.0.1/axion-linux-amd64-0.0.1.tgz
tar -xvzf axion-linux-amd64-0.0.1.tgz
mv axion ~/go/bin/
```

**From Source:**
```
git clone --depth 1 https://github.com/mrmahile/axion.git
cd axion; go install
```

## Configuration

The tool uses a configuration file located at `~/.config/axion/config.yaml`. On first run, if the file doesn't exist, you'll need to create it manually.

### Configuration File Structure

```yaml
default_remote_location: "/root"

credentials:
  - name: "worker1"
    # Optional: friendly VPS name
    ip: "192.168.1.1"
    username: "root"
    password: "yourpassword"

  - name: "worker2"
    # Optional: friendly VPS name
    ip: "192.168.1.2"
    username: "admin"
    password: "anotherpassword"

  - ip: "192.168.1.3"
    # Name field is optional - IP-only entries work too
    username: "root"
    password: "anotherpassword"
```

**Note:** The `name` field is optional. You can use either IP addresses or VPS names (or both). If a VPS name is provided, you can reference the server using the number in its name (e.g., `worker60` → index `60`). The tool matches VPS by extracting the numeric part from their names, so entries don't need to be in sequential order.

### Manual Configuration

You can manually create or edit the config file:

```bash
mkdir -p ~/.config/axion
nano ~/.config/axion/config.yaml
chmod 600 ~/.config/axion/config.yaml
```

## Usage

### Single VPS

Execute a command on a single VPS by number:

```bash
axion -i 42 -c "uptime"
```

### Multiple Selected VPS

Execute a command on multiple specific VPS (comma-separated):

```bash
axion -i 52,42,53,56,61,64 -c "tmux ls"
```

### Range of VPS

Execute a command on multiple VPS instances in a range:

```bash
axion -l 1-20 -c "apt install nginx -y"
```

## Options

- `-i <id>` - Run command on VPS by number. Supports single number or comma-separated list (e.g., `42` or `52,42,53`)
- `-l <range>` - Run command on multiple VPS in a range (e.g., `1-20`)
- `-c "<command>"` - Command to execute (required)
- `-silent` - Silent mode. Suppresses banner output
- `-version` - Print the version of the tool and exit

## Validation

- Either `-i` or `-l` must be provided (not both)
- `-c` must be non-empty
- VPS numbers are matched by the number in their name (e.g., `worker60` matches index `60`)

## Output Format

### Single VPS

```
[worker60] SUCCESS
STDOUT:
<output>
STDERR:
<error if any>
```

### Multiple VPS

```
[worker60] SUCCESS
STDOUT:
<output>

[worker61] FAILED
STDERR:
<error>
```

## Security

- Protect config file: `chmod 600 ~/.config/axion/config.yaml`
- Passwords are not logged
- SSH key authentication (via `secret` field) is planned for future enhancement

## Examples

```bash
# Update packages on VPS #42
axion -i 42 -c "apt update"

# Install nginx on VPS #1-20
axion -l 1-20 -c "apt install nginx -y"

# Check disk usage on multiple selected VPS
axion -i 52,42,53,56,61,64 -c "df -h"

# Restart service on multiple VPS
axion -l 1-10 -c "systemctl restart nginx"

# Check tmux sessions on specific VPS
axion -i 52,42,53,56,61,64 -c "tmux ls"

# Run command in silent mode (no banner)
axion -silent -i 42 -c "uptime"

# Check version
axion -version
```

## How It Works

The tool matches VPS entries by extracting the numeric part from their names:
- `worker60` → matches index `60`
- `worker42` → matches index `42`
- `server100` → matches index `100`

This means you can reference VPS by their logical numbers even if they're not in sequential order in your config file.
