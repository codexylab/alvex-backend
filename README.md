# ALVEX Backend

> Go backend for the ALVEX Enterprise AI Management Platform.

## Quick Start

### Prerequisites
- [Go 1.22+](https://go.dev/dl/)
- [PostgreSQL 16+](https://www.postgresql.org/download/)

### Setup

```bash
# 1. Copy environment config
cp .env.example .env
# Edit .env and fill in DATABASE_URL, JWT_SECRET, etc.

# 2. Install dependencies
go mod download

# 3. Run database migrations
make migrate-up

# 4. Seed the admin user
make seed

# 5. Start the development server
make run
# Server runs at http://localhost:8080
```

### Default Admin Login
```
Email:    admin@alvex.ai
Password: Admin@Alvex2024!
```
> **Change this password immediately after first login.**

---

## API Reference

### Base URL
```
http://localhost:8080/api/v1
```

### Authentication
All protected endpoints require a Bearer JWT token in the `Authorization` header:
```
Authorization: Bearer <token>
```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/login` | Login → returns JWT token |
| `GET`  | `/auth/me` | Current user profile |
| `GET`  | `/clients` | List clients (search, filter, paginate) |
| `POST` | `/clients` | Create new client |
| `GET`  | `/clients/:id` | Get client detail |
| `PUT`  | `/clients/:id` | Update client config |
| `PATCH`| `/clients/:id/status` | Toggle Active/Suspended |
| `DELETE`| `/clients/:id` | Delete client |
| `GET`  | `/billing/stats` | MRR and billing metrics |
| `GET`  | `/billing/invoices` | Invoice list |
| `POST` | `/billing/invoices` | Create invoice |
| `PATCH`| `/billing/invoices/:id/pay` | Mark invoice as paid |
| `GET`  | `/analytics/overview` | Dashboard stats |
| `GET`  | `/analytics/trends?period=7d` | Chart data |
| `GET`  | `/analytics/activity` | Live activity feed |

### WebSocket
```
ws://localhost:8080/ws/activity
```
Connect with a valid JWT token. Receives JSON activity events in real time.

### Webhooks (Public)
```
GET  /webhook/wa/v2/:clientId   → WhatsApp verification
POST /webhook/wa/v2/:clientId   → Incoming WhatsApp message
POST /webhook/chat/:clientId    → Incoming web chat message
```

---

## Project Structure

```
alvex-backend/
├── cmd/server/main.go         # Entry point
├── cmd/seed/main.go           # Admin seeder
├── internal/
│   ├── config/                # Environment config
│   ├── database/              # DB connection + migrations
│   ├── handlers/              # HTTP handlers
│   ├── middleware/            # Auth, logger
│   ├── models/                # Data models
│   ├── router/                # Route registration
│   └── services/ai/           # AI provider clients
└── pkg/
    ├── crypto/                # Key generation utilities
    └── response/              # Standard JSON responses
```

## License
MIT — CodexyLab
