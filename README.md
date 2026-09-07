# ALVEX Backend

Go API for the ALVEX multi-tenant AI assistant platform.

## Production architecture

- Runtime: Railway
- Database: Railway PostgreSQL
- Authentication: Supabase Auth
- Frontend: Vercel
- Billing: Stripe

## Quick Start

### Prerequisites

- Go 1.26+
- PostgreSQL 17+ (SQLite remains available for local development)

### Setup

```bash
# 1. Copy environment config
cp .env.example .env
# Edit .env and configure DATABASE_URL and the required secrets.

# 2. Install dependencies
go mod download

# 3. Run database migrations
make migrate-up

# 4. Start the development server
make run
# Server runs at http://localhost:8080
```

## API Reference

### Base URL
```
http://localhost:8080/api/v1
```

### Authentication
Protected endpoints require a Supabase access token in the `Authorization` header:
```
Authorization: Bearer <token>
```

Machine integrations use a separately issued scoped key:
```
X-API-Key: alvx_sk_<secret>
```
Only the key hash is stored. The raw secret is returned once when the key is created.
Machine routes are throttled independently per credential.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/auth/me` | Current user profile |
| `POST` | `/self-service/checkout` | Create an authenticated Stripe Checkout session |
| `POST` | `/platform/clients` | Super-admin client invitation and provisioning |
| `GET`  | `/api-keys/` | List machine-key metadata |
| `POST` | `/api-keys/` | Create a scoped machine key (secret returned once) |
| `DELETE` | `/api-keys/:id` | Revoke a machine key |
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
| `GET`  | `/integrations/clients/:id` | Scoped machine client read |
| `PUT`  | `/integrations/clients/:id` | Scoped machine client update |

### WebSocket
```
ws://localhost:8080/ws/activity
```
Connect with a valid Supabase token. The query-string token is supported only
for this WebSocket endpoint because browser WebSocket clients cannot set an
`Authorization` header.

### Webhooks (Public)
```
GET  /webhook/wa/v2/:clientId   → WhatsApp verification
POST /webhook/wa/v2/:clientId   → Incoming WhatsApp message
POST /webhooks/stripe           → Signed Stripe billing events
POST /widget/v1/:clientId/bootstrap → Create an origin-bound widget session
POST /widget/v1/:clientId/messages  → Send an authenticated widget message
GET  /widget/v1/:clientId/history   → Read the authenticated session history
```

Client portal endpoints under `/api/v1/client-portal` require both a valid
Supabase access token and an active `client_memberships` record.

---

## Project Structure

```
alvex-backend/
├── cmd/server/                # API process
├── cmd/migrate/               # Canonical migration command
├── pkg/config/                # Environment configuration and validation
├── pkg/database/              # Connections and ordered migrations
├── pkg/handlers/              # HTTP transport layer
├── pkg/middleware/            # Authentication and request middleware
├── pkg/models/                # Domain models and request contracts
├── pkg/repository/            # Persistence interfaces and SQL adapters
├── pkg/services/              # Business logic and provider integrations
└── pkg/router/                # Dependency wiring and route registration
```

## Deployment

Railway executes `/app/alvex-migrate` before starting `/app/alvex-api`.
The readiness endpoint is `/health/ready`; liveness is `/health/live`.

Set `PUBLIC_API_URL` to the public HTTPS Railway service URL. This value is
included in generated widget snippets; it must not be the Railway private URL.

## Public widget security

The Vercel-hosted widget starts with `POST /widget/v1/{clientId}/bootstrap`.
The backend verifies the browser `Origin` against the client's exact
`allowed_origins` entries (or its HTTPS domain when the list is empty), then
returns a short-lived opaque session token. Only the SHA-256 token digest is
stored. Chat, history, tickets, reactions, leads, typing events, and revocation
all require the token and the same origin.

## Durable provider jobs

Stripe and WhatsApp handlers verify signatures and persist unique provider
events to `background_jobs` before acknowledging the provider. Database workers
claim jobs with leases, retry temporary failures with exponential backoff, and
move exhausted work to the `dead` state for operational review. WhatsApp
webhooks process every entry/change/message, bind Meta `phone_number_id` to the
target client, send the generated reply through the Cloud API, and persist
outbound delivery status updates.

Set `ALERT_WEBHOOK_URL` to receive a payload-free JSON notification whenever a
worker exhausts a job's retries. Alerts contain job identity, type, attempt
counts, environment, and a generic failure summary; customer job payloads and
raw errors are never included.

Successful chat, conversation-summary, and FAQ-generation calls write immutable
provider/model/token counts to `ai_usage_events`, attributed to both the
organization and client. This token ledger is the stable input for a future
versioned pricing catalog; request-time code does not guess monetary cost from
provider rates that can change independently of ALVEX.

The super-admin `GET /api/v1/platform/operations` snapshot includes queue and
database health plus rolling 24-hour AI usage grouped by provider, model, and
operation.

Embed code uses both public deployment URLs:

```html
<script
  src="https://your-vercel-domain.example/widget.js"
  data-id="asst_your-client-id"
  data-api-url="https://your-railway-api.example"
  async
></script>
```

## License
MIT — CodexyLab
