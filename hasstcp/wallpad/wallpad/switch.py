import paramiko
import re
import sys

# ============= 사용자 설정 =============
SSH_HOST = "192.168.0.60"
SSH_PORT = 22
SSH_USER = "root"
SSH_PASS = "1!@Honami"

OIFNAME = "eth0.2"
CHAIN_NAME = "srcnat_wan"
TABLE_NAME = "fw4"
FAMILY = "inet"

INTERNAL_IP = "192.168.0.45"
ALLOWED_SNAT_IPS = ["10.9.1.147", "10.9.12.11", "10.9.13.22","10.9.13.147","10.9.1.18","10.9.1.17","10.9.1.28","10.9.21.22"]
# =======================================

def run_ssh_cmd(ssh, cmd):
    stdin, stdout, stderr = ssh.exec_command(cmd)
    out = stdout.read().decode()
    err = stderr.read().decode()
    if err and "Command failed" not in err and not out:
        raise RuntimeError(f"Command failed: {cmd}\nError: {err}")
    return out

def find_existing_handle(ssh):
    output = run_ssh_cmd(ssh, f"nft --handle list chain {FAMILY} {TABLE_NAME} {CHAIN_NAME}")
    pattern = rf"ip saddr {re.escape(INTERNAL_IP)} .* snat ip to .* # handle (\d+)"
    match = re.search(pattern, output)
    return match.group(1) if match else None

def delete_rule(ssh, handle):
    run_ssh_cmd(ssh, f"nft delete rule {FAMILY} {TABLE_NAME} {CHAIN_NAME} handle {handle}")
    print(f"[✓] Deleted previous SNAT rule with handle {handle}")

def add_rule(ssh, snat_ip):
    run_ssh_cmd(
        ssh,
        f"nft insert rule {FAMILY} {TABLE_NAME} {CHAIN_NAME} "
        f"ip saddr {INTERNAL_IP} oifname \"{OIFNAME}\" snat ip to {snat_ip}"
    )
    print(f"[✓] Added new SNAT rule: {INTERNAL_IP} → {snat_ip}")

def main():
    if len(sys.argv) != 2:
        print(f"Usage: python3 {sys.argv[0]} <snat_ip>")
        print(f"Allowed IPs: {', '.join(ALLOWED_SNAT_IPS)}")
        sys.exit(1)

    snat_ip = sys.argv[1]
    if snat_ip not in ALLOWED_SNAT_IPS:
        print(f"[!] Error: '{snat_ip}' is not in the allowed list: {ALLOWED_SNAT_IPS}")
        sys.exit(1)

    print(f"[INFO] Connecting to {SSH_HOST} to apply SNAT {INTERNAL_IP} → {snat_ip}...")

    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    ssh.connect(SSH_HOST, port=SSH_PORT, username=SSH_USER, password=SSH_PASS)

    try:
        handle = find_existing_handle(ssh)
        if handle:
            delete_rule(ssh, handle)
        add_rule(ssh, snat_ip)
    finally:
        ssh.close()

if __name__ == "__main__":
    main()
