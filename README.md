# Gender Classify API

A REST API that classifies names by gender, age, and nationality — built for HNG Internship 14, Backend Track.

It wraps [Genderize.io](https://genderize.io), [Agify.io](https://agify.io), and [Nationalize.io](https://nationalize.io) to generate enriched demographic profiles, stored in PostgreSQL.

---

## Getting Started

### Prerequisites

- Go 1.26+
- PostgreSQL

### Setup

1. Clone and enter the repo:

```bash
git clone https://github.com/abdulsalamcodes/hng14-s0-gender-classify.git
cd hng14-s0-gender-classify
```

2. Create a `.env` file:

```env
DATABASE_URL=postgres://user:password@localhost:5432/yourdb
```

3. Build and run:

```bash
go build -o bin/api ./cmd/api
./bin/api
```

Or run directly:

```bash
go run ./cmd/api
```

Server starts on port `8080`.

### Seed Database

To populate the database with sample profiles:

```bash
# First, ensure the table has the correct schema (run if table exists without country_name column):
psql $DATABASE_URL -c "ALTER TABLE profiles ADD COLUMN country_name VARCHAR(255) NOT NULL DEFAULT '';"
psql $DATABASE_URL -c "ALTER TABLE profiles ADD CONSTRAINT profiles_name_key UNIQUE (name);"

# Then seed:
go run ./cmd/seed
```

Seeding is **idempotent** — safe to run multiple times (duplicates are skipped).

---

## Database Schema

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID | Primary key |
| `name` | VARCHAR(255) | Unique person's full name |
| `gender` | VARCHAR(10) | "male" or "female" |
| `gender_probability` | FLOAT | Confidence score (0-1) |
| `sample_size` | INT | Data points used for prediction |
| `age` | INT | Exact age |
| `age_group` | VARCHAR(20) | child, teenager, adult, senior |
| `country_id` | VARCHAR(2) | ISO country code (NG, BJ, etc.) |
| `country_name` | VARCHAR(255) | Full country name |
| `country_probability` | FLOAT | Confidence score (0-1) |
| `created_at` | TIMESTAMP | Auto-generated |

---

## Endpoints

### `GET /`

Returns API metadata.

**Response (200)**

```json
{
  "name": "Gender Classify API",
  "author": "Abdulsalam Elhakamy",
  "version": "1.0.0",
  "usage": "GET /api/classify?name=<name> | POST /api/profile"
}
```

---

### `GET /api/classify`

Classify a name by gender using Genderize.io.

**Query parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | Yes | The name to classify |

**Example**

```
GET /api/classify?name=james
```

**Response (200)**

```json
{
  "status": "success",
  "data": {
    "name": "james",
    "gender": "male",
    "probability": 0.99,
    "sample_size": 1234,
    "is_confident": true,
    "processed_at": "2026-04-15T10:00:00Z"
  }
}
```

---

### `POST /api/profiles`

Create a demographic profile for a name. Calls Genderize, Agify, and Nationalize concurrently, then persists the result. **Idempotent** — returns the existing profile if one already exists.

**Request body**

```json
{ "name": "james" }
```

**Response (201)**

```json
{
  "status": "success",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "james",
    "gender": "male",
    "gender_probability": 0.99,
    "sample_size": 1234,
    "age": 62,
    "age_group": "senior",
    "country_id": "US",
    "country_name": "United States",
    "country_probability": 0.08,
    "created_at": "2026-04-15T10:00:00Z"
  }
}
```

---

### `GET /api/profiles`

List all profiles with filtering, sorting, and pagination.

**Filtering Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `gender` | string | Filter by gender (male/female) |
| `age_group` | string | Filter by age group (child/teenager/adult/senior) |
| `country_id` | string | Filter by country ISO code |
| `min_age` | int | Minimum age |
| `max_age` | int | Maximum age |
| `min_gender_probability` | float | Minimum gender probability (0-1) |
| `min_country_probability` | float | Minimum country probability (0-1) |

**Sorting Parameters**

| Parameter | Values | Default |
|-----------|--------|---------|
| `sort_by` | `age`, `created_at`, `gender_probability` | `created_at` |
| `order` | `asc`, `desc` | `desc` |

**Pagination Parameters**

| Parameter | Default | Max | Description |
|-----------|---------|-----|-------------|
| `page` | 1 | - | Page number |
| `limit` | 10 | 50 | Items per page |

**Response (200)**

```json
{
  "status": "success",
  "page": 1,
  "limit": 10,
  "total": 2026,
  "data": [...]
}
```

**Example Requests**

```bash
# Filter by gender and country
curl "http://localhost:8080/api/profiles?gender=male&country_id=NG"

# Age range with sorting
curl "http://localhost:8080/api/profiles?min_age=25&max_age=40&sort_by=age&order=desc"

# Probability filters
curl "http://localhost:8080/api/profiles?min_gender_probability=0.8&min_country_probability=0.5"

# Combined filters, sorting, and pagination
curl "http://localhost:8080/api/profiles?gender=female&country_id=KE&min_age=20&sort_by=gender_probability&order=desc&page=2&limit=25"
```

---

### `GET /api/profiles/search`

Natural language search — convert plain English queries into filters.

**Query Parameters**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Natural language query |
| `page` | int | No | Page number (default: 1) |
| `limit` | int | No | Items per page (default: 10, max: 50) |

**Supported Patterns**

| Pattern | Mapping |
|---------|---------|
| `male`, `males`, `men`, `man`, `boys` | gender=male |
| `female`, `females`, `women`, `woman`, `girls` | gender=female |
| `child`, `children`, `kid`, `kids` | age_group=child |
| `teenager`, `teens`, `teen` | age_group=teenager |
| `adult`, `adults`, `grown` | age_group=adult |
| `senior`, `elder`, `elderly`, `old` | age_group=senior |
| `young`, `youth` | age=16-24 |
| `above 30`, `over 30`, `older than 30` | min_age=30 |
| `below 30`, `under 30`, `younger than 30` | max_age=30 |
| `from Nigeria`, `in Kenya` | country_id=NG, country_id=KE |

**Example Queries**

```bash
# Young males
curl "http://localhost:8080/api/profiles/search?q=young+males"

# Females above 30
curl "http://localhost:8080/api/profiles/search?q=females+above+30"

# People from Angola
curl "http://localhost:8080/api/profiles/search?q=people+from+angola"

# Adult males from Kenya
curl "http://localhost:8080/api/profiles/search?q=adult+males+from+kenya"

# Teenagers above 17
curl "http://localhost:8080/api/profiles/search?q=teenagers+above+17"

# With pagination
curl "http://localhost:8080/api/profiles/search?q=males+from+ghana&page=2&limit=20"
```

**Response (200)**

```json
{
  "status": "success",
  "page": 1,
  "limit": 10,
  "total": 45,
  "data": [...]
}
```

---

### `GET /api/profiles/{id}`

Retrieve a single profile by UUID.

**Response (200)** — same shape as `POST /api/profiles`

**Error (404)** — Profile not found

---

### `DELETE /api/profiles/{id}`

Delete a profile by UUID.

**Response (204)** — No content

**Error (404)** — Profile not found

---

## Error Responses

All errors share the same shape:

```json
{
  "status": "error",
  "message": "Description of what went wrong"
}
```

| Scenario | Status Code |
|----------|-------------|
| Missing or invalid parameter | `400 Bad Request` |
| No prediction available for name | `404 Not Found` |
| Profile not found | `404 Not Found` |
| External API unreachable | `502 Bad Gateway` |
| Database error | `500 Internal Server Error` |

---

## Project Structure

```
├── cmd/
│   ├── api/main.go           # API server entry point
│   └── seed/main.go          # Database seeding script
├── internal/
│   ├── config/               # Configuration handling
│   ├── data/                 # Seed data (seed_profiles.json)
│   ├── handlers/             # HTTP handlers
│   ├── models/               # Data models and types
│   ├── repository/           # Database operations
│   └── services/             # Business logic and NL search
├── pkg/
│   └── api/                  # External API clients
├── bin/                      # Compiled binaries
├── go.mod
└── go.sum
```

---

## Tech Stack

- **Language:** Go 1.26+
- **Router:** Chi v5
- **Database:** PostgreSQL (via pgx/v5)
- **Auth:** GitHub OAuth 2.0 + PKCE, JWT (golang-jwt/jwt/v5)
- **Rate limiting:** go-chi/httprate
- **External APIs:** [Genderize.io](https://genderize.io), [Agify.io](https://agify.io), [Nationalize.io](https://nationalize.io)

---

## Stage 3 — Authentication & Access Control

### System Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   CLI Tool      │    │  Web Portal     │    │  Direct API     │
│ (Bearer token)  │    │ (HTTP-only cookie│    │ (Bearer token)  │
└────────┬────────┘    └────────┬────────┘    └────────┬────────┘
         │                     │                       │
         └─────────────────────▼───────────────────────┘
                        ┌──────────────┐
                        │  Backend API │
                        │  :8080       │
                        └──────┬───────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
        ┌──────────┐   ┌─────────────┐   ┌──────────────┐
        │ GitHub   │   │  PostgreSQL │   │ Genderize /  │
        │  OAuth   │   │   (users,   │   │ Agify /      │
        │          │   │   tokens,   │   │ Nationalize  │
        └──────────┘   │   profiles) │   └──────────────┘
                       └─────────────┘
```

### Authentication Flow

#### Web Portal Flow
1. User clicks "Continue with GitHub" → browser redirects to `GET /auth/github?source=web`
2. Backend redirects to GitHub OAuth
3. GitHub redirects to `GET /auth/github/callback?code=...&state=...`
4. Backend exchanges code for GitHub access token, fetches user profile
5. Backend sets **HTTP-only cookies** (`access_token`, `refresh_token`, `csrf_token`)
6. Browser redirects to portal dashboard

#### CLI Flow (PKCE)
1. CLI generates `code_verifier` (random 32 bytes, base64url encoded)
2. CLI computes `code_challenge = BASE64URL(SHA256(code_verifier))`
3. CLI starts a temporary local HTTP server on a random port
4. Browser opens to `GET /auth/github?source=cli&code_challenge=...&cli_redirect_uri=http://localhost:<port>/callback`
5. Backend stores state → {challenge, cli_redirect_uri}; redirects to GitHub
6. GitHub redirects to backend callback
7. Backend (seeing source=cli) redirects to `http://localhost:<port>/callback?code=...`
8. CLI local server captures the `code`
9. CLI POSTs `{code, code_verifier}` to `POST /auth/cli-exchange`
10. Backend exchanges with GitHub (PKCE verified), issues tokens, returns JSON
11. CLI stores tokens at `~/.insighta/credentials.json`

### Token Handling

| Token | Duration | Storage | Purpose |
|-------|----------|---------|---------|
| Access token | **3 minutes** | JWT (signed HS256) | Authenticate API requests |
| Refresh token | **5 minutes** | Random bytes (hashed SHA-256 in DB) | Obtain new token pair |

**Token rotation:** Each refresh invalidates the old refresh token immediately and issues a new pair. Using an already-consumed refresh token returns 401.

**Revocation:** Logout deletes all refresh tokens from the DB, making every active session immediately invalid.

### Role Enforcement Logic

```
Request → LoggingMiddleware → CORS → Recoverer
       → /auth/* : AuthRateLimit (10/min/IP)
       → /api/*  : APIVersionMiddleware (X-API-Version: 1 required)
                 → APIRateLimit (60/min/user)
                 → RequireAuth (validates JWT, checks is_active in DB)
                 → handler
                    → RequireRole("admin") on POST /api/profiles
                    → RequireRole("admin") on DELETE /api/profiles/:id
```

| Role | Permissions |
|------|-------------|
| `analyst` (default) | Read all profiles, search, export |
| `admin` | All analyst permissions + create + delete |

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `GITHUB_CLIENT_ID` | Yes | OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | Yes | OAuth App client secret |
| `GITHUB_REDIRECT_URL` | Yes | Must match OAuth App callback URL |
| `JWT_SECRET` | Yes | Access token signing key (use `openssl rand -hex 32`) |
| `JWT_REFRESH_SECRET` | Yes | Refresh token signing key |
| `FRONTEND_URL` | Yes | Portal URL for post-OAuth redirect |
| `PORT` | No | Server port (default: 8080) |

### Auth Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/auth/github` | — | Start OAuth flow |
| GET | `/auth/github/callback` | — | GitHub OAuth callback |
| POST | `/auth/cli-exchange` | — | CLI PKCE token exchange |
| POST | `/auth/refresh` | — | Rotate token pair |
| POST | `/auth/logout` | Required | Revoke tokens |
| GET | `/api/whoami` | Required | Current user info |

### Rate Limits

| Scope | Limit | Key |
|-------|-------|-----|
| `/auth/*` | 10 req/min | IP address |
| `/api/*` | 60 req/min | User ID (JWT) |

### Project Structure (Stage 3 additions)

```
internal/
├── handlers/
│   ├── handlers.go          # Profile CRUD + export + whoami
│   └── auth_handlers.go     # OAuth flow, token management
├── middleware/
│   ├── middleware.go        # RequireAuth, RequireRole, APIVersionMiddleware
│   ├── ratelimit.go         # AuthRateLimit, APIRateLimit
│   ├── logging.go           # Request logger
│   └── cors.go              # CORS for portal cross-origin requests
├── services/
│   ├── auth.go              # GitHub OAuth, token pair issuance/rotation
│   └── tokens.go            # JWT issue/validate, refresh token generation
└── repository/
    └── repository.go        # users + refresh_tokens tables + queries
```