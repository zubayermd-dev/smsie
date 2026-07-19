<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/License-GPL%203.0-blue?style=for-the-badge" alt="License">
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-ffd43b?style=for-the-badge&logo=linux&logoColor=white" alt="Platform">
</p>

<h1 align="center">Ivy</h1>

<p align="center">
  <strong>SMS Management Dashboard with Telegram Integration, Bangla Support & Modern UI</strong>
</p>

<p align="center">
  A feature-rich SMS management platform for GSM/LTE modems. Send and receive SMS, integrate with Telegram, manage conversations, and monitor modems — all from a beautiful, dark-mode-enabled web interface.
</p>

---

## What is Ivy?

Ivy is a **fork of [smsie](https://github.com/pccr10001/smsie)** — an open-source SMS management dashboard written in Go. While the original smsie provides solid modem management and SMS operations, Ivy takes it further with:

- **Telegram Bot Integration** — Send and receive SMS directly from Telegram
- **Bangla/Bengali Language Support** — Full Unicode rendering with Noto Sans Bengali font
- **Modern Redesigned UI** — Dark mode, conversation threads, real-time updates, and compose functionality
- **Security Hardened** — XSS protection, permission fixes, input validation, and secure secret management
- **Huawei Modem Support** — Extended compatibility beyond Quectel modules

Ivy is designed for developers, hobbyists, and anyone who needs a self-hosted SMS gateway with a modern interface.

---

## Features

### Core Features (from smsie)
- **Modem Management** — Auto-detect and monitor GSM/LTE modems with signal strength, operator info, and registration status
- **SMS Operations** — Read, send, and manage SMS messages with PDU encoding support
- **AT Command Terminal** — Execute raw AT commands directly on modems
- **Voice Calling** — WebRTC-based voice calls with DTMF support (Quectel UAC)
- **SIP Client** — Per-modem SIP integration for VoIP connectivity
- **Webhooks** — Forward SMS to Telegram, Slack, or custom endpoints
- **User Management** — Role-based access control with API key support
- **MCP Server** — Model Context Protocol integration for AI assistants

### New in Ivy
- **Telegram Bot** — Two-way SMS via Telegram with `/send`, `/status`, and `/help` commands
- **Conversation View** — Group messages by phone number with unread counts
- **Dark Mode** — Full dark theme with persistent toggle
- **Compose Modal** — Send SMS directly from the inbox with modem selection
- **Type Filtering** — Filter messages by All/Received/Sent
- **Search** — Search messages by phone number or content
- **Auto-Refresh** — Toggle live updates every 5 seconds
- **Mark as Read** — Messages marked as read when viewed
- **Delete Messages** — Delete individual messages or entire conversations
- **Bangla Font** — Proper Bengali text rendering with Noto Sans Bengali
- **Click to Expand** — Full message view in modal for long SMS
- **Sent/Received Badges** — Visual indicators with arrow icons
- **Huawei Modem Support** — Extended ICCID detection for Huawei EG162G and similar
- **SMS Deduplication** — Prevents duplicate webhook fires
- **Security Fixes** — XSS protection, JWT secrets, input validation, permission checks

---

## Screenshots

> **Note:** Screenshots coming soon. The UI features a glassmorphism design with light/dark mode support.

---

## Download

### Build from Source

```bash
git clone https://github.com/zubayermd-dev/ivy.git
cd ivy
go mod tidy
go build -o ivy .
```

### Pre-built Binaries

Download the latest release from [Releases](https://github.com/zubayermd-dev/ivy/releases).

---

## Setup Guide

### Prerequisites

- **Go 1.25+**
- **Serial Drivers** for your modem
- **Linux:** `sudo apt install portaudio19-dev libusb-1.0-0-dev ffmpeg`

### Quick Start

1. **Clone and build:**
   ```bash
   git clone https://github.com/zubayermd-dev/ivy.git
   cd ivy
   go build -o ivy .
   ```

2. **Configure environment (optional):**
   ```bash
   cp .env.example .env
   # Edit .env with your settings
   ```

3. **Run the server:**
   ```bash
   ./ivy
   ```

4. **Access the dashboard:**
   Open `http://localhost:8080` in your browser.

5. **First login:**
   - Username: `admin`
   - Check logs for the auto-generated password, or check `/opt/ivy/.initial_admin_password`

### Telegram Bot Setup

1. Create a bot via [@BotFather](https://t.me/BotFather) on Telegram
2. Get your chat ID via [@userinfobot](https://t.me/userinfobot)
3. Add to `.env`:
   ```
   TG_BOT_TOKEN=your_bot_token
   TG_CHAT_ID=your_chat_id
   IVY_PASS=your_admin_password
   IVY_ICCID=your_modem_iccid
   ```
4. Run the bot:
   ```bash
   python3 telegram_bot.py
   ```

### Systemd Deployment

```bash
sudo cp ivy /opt/ivy/
sudo cp .env /opt/ivy/
sudo cp ivy.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable ivy
sudo systemctl start ivy
```

---

## API Reference

### Authentication

```bash
# Login
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your_password"}'

# Use the returned token
TOKEN=<jwt_token>
```

### Key Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/modems` | List all modems |
| `GET` | `/api/v1/sms` | List SMS messages |
| `POST` | `/api/v1/modems/:iccid/send` | Send SMS |
| `DELETE` | `/api/v1/sms/:id` | Delete SMS |
| `DELETE` | `/api/v1/sms/phone?phone=X` | Delete conversation |
| `POST` | `/api/v1/sms/read` | Mark messages as read |
| `POST` | `/api/v1/modems/:iccid/at` | Execute AT command |

### Example: Send SMS

```bash
curl -X POST http://localhost:8080/api/v1/modems/IMSI_XXXXXXXX/send \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phone":"+8801711111111","message":"Hello from Ivy!"}'
```

---

## Telegram Bot Commands

| Command | Description |
|---------|-------------|
| `/send +phone message` | Send SMS via modem |
| `/status` | Check modem status |
| `/help` | Show available commands |

---

## Supported Modems

Ivy works with **any GSM/LTE modem** that supports standard AT commands. The modem is auto-detected when plugged in — no configuration needed.

### Tested Modems

| Modem | SMS | Voice | Connection | Notes |
|-------|-----|-------|------------|-------|
| Quectel EC20 | ✅ | ✅ | LTE | Full UAC support |
| Quectel EC800M | ✅ | ✅ | LTE | Full UAC support |
| OpenLuat Air780E | ✅ | ✅ | LTE | Full UAC support |
| Huawei EG162G | ✅ | ❌ | EDGE | 2.5G modem |
| Huawei E161/E169 | ✅ | ❌ | HSDPA | 3.5G modem |

### Compatibility

Any modem that responds to these AT commands should work:
- `AT` — Basic communication test
- `AT+ICCID` or `AT^ICCID?` — SIM card identification
- `AT+CSQ` — Signal quality
- `AT+CMGF` — SMS mode (PDU/Text)
- `AT+CMGS` — Send SMS
- `AT+CMGL` — List SMS

**Common compatible brands:** Quectel, Huawei, ZTE, SIMCom, Sierra Wireless, Teltonika, u-blox, OpenLuat, and most Chinese 4G/LTE modules (EC20, EC25, MC60, A7670, etc.).

---

## What's Changed from smsie

| Area | Original smsie | Ivy |
|------|-------|-----|
| **UI** | Basic list view | Conversation threads, dark mode, compose modal |
| **Font** | Latin only | Latin + Bangla (Noto Sans Bengali) |
| **Telegram** | Webhook only | Two-way bot with commands |
| **Security** | Basic auth | JWT auto-gen, XSS fixes, input validation |
| **Delete** | Not available | Single + bulk delete |
| **Search** | Not available | Full-text search |
| **Read Status** | Not tracked | Unread counts, mark as read |
| **Modem Support** | Quectel only | Quectel + Huawei |
| **Deduplication** | Not present | 1-minute dedup window |

---

## Credits

**Ivy** is a fork of [smsie](https://github.com/pccr10001/smsie) by [pccr10001](https://github.com/pccr10001).

Original smsie provides the core modem management, SMS operations, voice calling, SIP integration, and webhook system. Ivy extends this foundation with UI improvements, security hardening, Telegram integration, and additional modem support.

### Original smsie Features Retained
- Modem auto-detection and monitoring
- SMS send/receive with PDU encoding
- AT command terminal
- Voice calling with WebRTC
- SIP client integration
- Webhook system
- User management with RBAC
- API key management
- MCP server support
- Docker deployment

---

## License

This project is licensed under the **GNU General Public License v3.0** — see the [LICENSE](LICENSE) file for details.

---

## Disclaimer

Ivy is an independent fork maintained separately from the original smsie project. While it builds upon smsie's codebase, the features, modifications, and support provided here are not affiliated with or endorsed by the original smsie authors.

---

<p align="center">
  Made with ❤️ for the open-source community
</p>
