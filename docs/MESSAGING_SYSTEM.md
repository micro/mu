# Messaging System Guide

Complete guide for mu's messaging system. Messages are delivered using mail/SMTP protocols, supporting both internal user-to-user messaging and external email communication.

## Quick Start

### Internal Messages Only (Default)
```bash
# No configuration needed
./mu --serve --address :8080
```
Users can send messages to each other by username. Messages stored locally, no external SMTP required.

### External Message Support
```bash
# Set SMTP port (optional, defaults to 2525)
export MAIL_PORT="2525"

./mu --serve --address :8080
```

The SMTP server always runs to receive mail. Configure DNS MX records to route mail to your server.

### Production Setup
```bash
# Use standard SMTP port
export MAIL_PORT="25"

# Set your mail domain
export MAIL_DOMAIN="yourdomain.com"

# DKIM signing (optional but recommended)
./scripts/generate-dkim-keys.sh yourdomain.com
export MAIL_SELECTOR="default"

./mu --serve --address :8080
```

---

## Table of Contents

1. [How It Works](#how-it-works)
2. [Environment Variables](#environment-variables)
3. [SMTP Server Setup (Receiving)](#smtp-server-setup)
4. [Message Delivery (Sending)](#smtp-client-setup)
5. [DKIM Setup](#dkim-setup)
6. [DNS Configuration](#dns-configuration)
7. [Testing](#testing)
8. [Troubleshooting](#troubleshooting)

---

## How It Works

### Sending Messages

**Internal recipient (username only):**
```
To: alice
→ Stored in mail.json
→ Appears in alice's inbox
```

**External recipient (email with @):**
```
To: bob@gmail.com
→ Sent via SMTP
→ Copy stored in sender's sent folder
```

### Receiving Messages

**SMTP Server (when enabled):**
```
Internet → SMTP Server (port 2525/25)
           ↓
       Validate User Exists?
           ↓ Yes
       Store in mail.json
           ↓
       User's Inbox
```

**Security:**
- Not an open relay
- Only accepts mail for existing mu users
- Rejects unknown recipients with "550 User not found"
- **Rate limiting:**
  - 10 connections per hour per IP address
  - 100 messages per day per sender email
- **SPF verification:** Checks sender domain SPF records (logs failures, doesn't reject)
- **Blocklist:** Block abusive senders by email or IP address

**Anti-spam protections:**
- Connection timeouts (10s read/write)
- Message size limit (10 MB)
- Recipient limit (50 per message)
- Automatic cleanup of rate limit tracking
- Admin interface for managing blocklist

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIL_PORT` | `2525` | Port for receiving mail (use 25 for production) |
| `MAIL_DOMAIN` | `localhost` | Domain for email addresses (e.g., user@domain.com) |
| `MAIL_SELECTOR` | `default` | DKIM selector (DNS record name) |

**SMTP server always runs** - no enable flag needed. Just configure DNS to route mail.

**Access Control:**
- Mail functionality is restricted to admins and members only
- Set user roles via the admin interface

**How Message Delivery Works:**
- Internal messages (username only) are stored directly in the local database
- External messages (@domain) are sent via SMTP protocol to recipient mail servers using MX lookups
- DKIM signing is applied automatically if configured
- No external SMTP relay needed - direct delivery to recipient servers

---

## SMTP Server Setup (Receiving Messages)

The messaging system uses SMTP protocol to receive messages from the internet.

### Configure Port

```bash
export MAIL_PORT="2525"  # Development
# export MAIL_PORT="25"   # Production
```

Default is `:2525` for local testing without requiring root privileges.

### Status Messages

**On startup:**
```
smtp: Starting SMTP server (receive only) on :2525
```

### How It Works

1. Receives email from internet
2. Extracts username from recipient (before @)
3. Validates user exists in mu
4. Stores message if valid, rejects if not
5. Sender's email preserved in message

**Example:**
```
Email to: alice@yourdomain.com
→ Looks up user: alice
→ If exists: stores message
→ If not: rejects with 550 error
```

### Not an Open Relay

The server only accepts mail for valid local users. It cannot be used to relay spam.

---

## Message Delivery (Sending)

The messaging system delivers messages using SMTP protocol. External messages are sent directly to recipient mail servers using MX lookups. No external relay required.

### How It Works

1. User sends message to external address (e.g., bob@gmail.com)
2. System looks up MX records for gmail.com
3. Connects directly to Gmail's mail servers via SMTP
4. Delivers message with DKIM signature (if configured)
5. Stores copy in sender's sent folder

### DKIM Signing

DKIM signatures are automatically added to outbound emails if keys are configured. This improves deliverability and prevents spam filtering.

```bash
# Generate keys
./scripts/generate-dkim-keys.sh yourdomain.com

# Configure
export MAIL_DOMAIN="yourdomain.com"
export MAIL_SELECTOR="default"
```

**No SMTP credentials needed** - Direct delivery handles everything.

---

## DKIM Setup

DKIM adds digital signatures to outbound messages to prove authenticity and improve deliverability.

### Why Use DKIM?

**Without DKIM:**
- Emails likely go to spam
- Gmail/Yahoo mark as suspicious
- Poor deliverability

**With DKIM:**
- Emails reach inbox
- 70%+ better deliverability
- Professional setup
- Required for DMARC

### Generate DKIM Keys

```bash
./scripts/generate-dkim-keys.sh yourdomain.com default
```

This creates:
- `~/.mu/keys/dkim.key` - Private key (keep secure!)
- `~/.mu/keys/dkim.pub` - Public key (reference)
- DNS TXT record output

### Manual Key Generation

```bash
mkdir -p ~/.mu/keys
chmod 700 ~/.mu/keys

openssl genrsa -out ~/.mu/keys/dkim.key 2048
chmod 600 ~/.mu/keys/dkim.key

# Extract public key for DNS
openssl rsa -in ~/.mu/keys/dkim.key -pubout -outform PEM | \
  grep -v "PUBLIC KEY" | tr -d '\n'
```

### Configure DKIM

```bash
export MAIL_DOMAIN="yourdomain.com"
export MAIL_SELECTOR="default"
```

**Note:** DKIM automatically enables if keys exist at `~/.mu/keys/dkim.key`. No other config needed.

### Status Messages

**Enabled:**
```
dkim: DKIM signing enabled for domain yourdomain.com with selector default
```

**Disabled:**
```
mail: DKIM signing disabled: DKIM private key not found at ~/.mu/keys/dkim.key
```

---

## DNS Configuration

### MX Records (Required for Receiving)

```dns
yourdomain.com.           IN  MX  10  mail.yourdomain.com.
mail.yourdomain.com.      IN  A       your.server.ip
```

### SPF Record (Recommended)

```dns
yourdomain.com.  IN  TXT  "v=spf1 a:mail.yourdomain.com ~all"
```

### DKIM Record (Recommended)

```dns
default._domainkey.yourdomain.com.  IN  TXT  "v=DKIM1; k=rsa; p=MIIBIjAN..."
```

Replace `MIIBIjAN...` with your public key from `generate-dkim-keys.sh` output.

### DMARC Record (Optional)

```dns
_dmarc.yourdomain.com.  IN  TXT  "v=DMARC1; p=quarantine; rua=mailto:dmarc@yourdomain.com"
```

### Verify DNS

```bash
# Check MX
dig MX yourdomain.com

# Check SPF
dig TXT yourdomain.com

# Check DKIM
dig TXT default._domainkey.yourdomain.com

# Check DMARC
dig TXT _dmarc.yourdomain.com
```

---

## Testing

### Test Receiving

```bash
# Using swaks
swaks --to alice@localhost --from test@example.com \
      --server localhost:2525 \
      --header "Subject: Test" --body "Hello"

# Using Python
python3 << 'EOF'
import smtplib
from email.mime.text import MIMEText

msg = MIMEText("Test message")
msg['Subject'] = 'Test'
msg['From'] = 'test@example.com'
msg['To'] = 'alice@localhost'

s = smtplib.SMTP('localhost', 2525)
s.send_message(msg)
s.quit()
print("Sent!")
EOF
```

### Test Sending

1. Login to mu web interface
2. Go to Mail → Compose
3. Send to external address (e.g., your Gmail)
4. Check recipient inbox

### Verify DKIM

**In Gmail:**
1. Open the email
2. Click "Show original" (⋮ menu)
3. Look for "DKIM: 'PASS'"

**Online Tools:**
- https://www.mail-tester.com/
- https://dkimvalidator.com/
- Send to: check-auth@verifier.port25.com

---

## Troubleshooting

### SMTP Server Not Starting

**Issue:** No "Starting SMTP server" message

**Check:**
1. Port already in use? Try a different port
2. Permission issue? Port 25 requires root/capabilities

**Test port availability:**
```bash
lsof -i :2525
```

### Mail to External Address Fails

**Issue:** "Failed to send email"

**Check:**
1. DNS resolving MX records correctly?
2. Firewall blocking outbound port 25?
3. Server IP not blacklisted?

**Test MX lookup:**
```bash
dig +short MX gmail.com
```

**Test connection:**
```bash
telnet mx.example.com 25
```

### DKIM Not Signing

**Issue:** No "DKIM signing enabled" message

**Check:**
1. Keys exist: `ls -la ~/.mu/keys/dkim.key`
2. Permissions: `chmod 600 ~/.mu/keys/dkim.key`
3. Valid key format (PEM)

### DKIM Fails Verification

**Issue:** "dkim=fail" in headers

**Common causes:**
1. DNS not propagated - wait 1-48 hours
2. Public/private key mismatch - regenerate both
3. Wrong selector - must match DNS record name
4. DNS format issues - no line breaks in public key

**Verify DNS:**
```bash
dig TXT default._domainkey.yourdomain.com +short
```

### Emails Going to Spam

**Common causes:**
- Missing or invalid SPF/DKIM/DMARC records
- Low sender reputation (new domain/IP)
- Content triggers spam filters
- Rate limited by recipient server

**Solutions:**
1. Verify all DNS records are correct
2. Start with low volume, gradually increase
3. Avoid spam trigger words ("free", "click here", etc.)
4. Use proper HTML formatting
5. Include unsubscribe links

---

## Blocklist Management

### Blocking Abusive Senders

Navigate to `/admin/blocklist` (admin access required) to manage blocked senders.

**Block by email:**
- Individual: `spammer@example.com`
- Domain wildcard: `*@spammydomain.com`

**Block by IP:**
- `192.168.1.100`

### Effects of Blocking

When a sender or IP is blocked:
- Connection rejected immediately with SMTP 554 error
- Message: "Transaction failed: sender blocked"
- No rate limiting check performed (blocked before that)
- Logged for monitoring

### Managing the Blocklist

**Via Admin Interface:**
1. Go to `/admin/blocklist`
2. Enter email or IP to block
3. Click "Block Email" or "Block IP"
4. View currently blocked entries
5. Click "Unblock" to remove

**Blocklist Storage:**
- Stored in `~/.mu/blocklist.json`
- Automatically loaded on startup
- Persisted immediately when changed

**Log Messages:**
```
smtp: Rejected blocked sender: spammer@bad.com (IP: 1.2.3.4)
mail: Blocked email: spammer@bad.com
mail: Unblocked email: reformed@example.com
```

---

**Solutions:**
1. Add DKIM signing
2. Configure SPF record
3. Set up DMARC
4. Use proper From address (@yourdomain.com)
5. Warm up IP (gradual sending increase)
6. Check blacklists: https://mxtoolbox.com/blacklists.aspx

### Permission Errors

```bash
# Fix DKIM key permissions
chmod 700 ~/.mu/keys
chmod 600 ~/.mu/keys/dkim.key
```

### SMTP Server on Port 25

**Issue:** Permission denied

**Fix:** Run as root or use CAP_NET_BIND_SERVICE:
```bash
sudo setcap 'cap_net_bind_service=+ep' ./mu
```

---

## Complete Production Setup

### 1. DNS Configuration

```bash
# Add these records to your DNS
yourdomain.com.  IN  MX  10  mail.yourdomain.com.
mail.yourdomain.com.  IN  A  your.server.ip
yourdomain.com.  IN  TXT  "v=spf1 a:mail.yourdomain.com ~all"
```

### 2. Generate DKIM Keys

```bash
./scripts/generate-dkim-keys.sh yourdomain.com default
```

Add the DNS TXT record from output.

### 3. Environment Variables

```bash
# Mail configuration
export MAIL_PORT="25"
export MAIL_DOMAIN="yourdomain.com"
export MAIL_SELECTOR="default"
```

### 4. Firewall

```bash
# Allow SMTP port
sudo ufw allow 25/tcp
```

### 5. Start Service

```bash
./mu --serve --address :8080
```

### 6. Verify

```bash
# Check logs
# Should see:
# - "DKIM signing enabled for domain yourdomain.com"
# - "Starting SMTP server (receive only) on :25"

# Test receiving
swaks --to user@yourdomain.com --from test@gmail.com \
      --server yourdomain.com --header "Subject: Test"

# Test sending (via web interface)
# Send to external address, verify DKIM passes
```

---

## Systemd Service

```ini
[Unit]
Description=Mu Mail Service
After=network.target

[Service]
Type=simple
User=mu
WorkingDirectory=/opt/mu

Environment="MAIL_PORT=25"
Environment="MAIL_DOMAIN=yourdomain.com"
Environment="MAIL_SELECTOR=default"

ExecStart=/opt/mu/mu --serve --address :8080
Restart=always

[Install]
WantedBy=multi-user.target
```

---

## Architecture Summary

**Components:**
- SMTP Server: Receives mail from internet (optional, disabled by default)
- SMTP Client: Sends mail to external servers (configurable)
- Internal Storage: JSON file for all messages

**Security Features:**
- Recipient validation (not an open relay)
- Rate limiting (per-IP and per-sender)
- SPF verification for incoming mail
- Connection timeouts and size limits
- Automatic rate limit cleanup
- Email and IP blocking with admin interface

**Message Flow:**
- DKIM Signing: Optional, auto-enabled if keys present

**Security:**
- Authentication required for sending
- SMTP server only accepts mail for valid users
- Not an open relay
- DKIM optional but recommended

**Configuration:**
- All in mail.Load() - automatic on startup
- Environment variables only
- No code changes needed

---

## See Also

- [Environment Variables](/docs/environment) - All environment variables
- Generate DKIM keys: `./scripts/generate-dkim-keys.sh`
- Test mail: https://www.mail-tester.com/
