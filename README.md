# hng14-s0-gender-classify

A REST API that classifies names by gender, age, and nationality — built for HNG Internship 14, Backend Track.

It wraps [Genderize.io](https://genderize.io), [Agify.io](https://agify.io), and [Nationalize.io](https://nationalize.io) to generate enriched demographic profiles, stored in PostgreSQL.

---

## Getting started

**Prerequisites:** Go 1.21+, PostgreSQL

1. Clone and enter the repo:

```bash
git clone https://github.com/abdulsalamcodes/hng14-s0-gender-classify.git
cd hng14-s0-gender-classify
```

2. Create a `.env` file:

```env
DATABASE_URL=postgres://user:password@localhost:5432/yourdb
```

3. Run:

```bash
go run main.go
```

Server starts on port `8080`.

---

## Endpoints

### `GET /`

Returns API metadata.

**Response (200)**

```json
{
  "author": "Abdulsalam Elhakamy",
  "name": "Gender Classify API",
  "usage": "GET /api/classify?name=<name> | POST /api/profile",
  "version": "1.0.0"
}
```

---

### `GET /api/classify`

Classify a name by gender using Genderize.io.

**Query parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | Yes | The name to classify |

**Example request**

```
GET /api/classify?name=james
```

**Success response (200)**

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

| Field | Description |
|-------|-------------|
| `gender` | `"male"` or `"female"` |
| `probability` | Confidence score from Genderize.io (0–1) |
| `sample_size` | Number of data points used for the prediction |
| `is_confident` | `true` when `probability >= 0.7` AND `sample_size >= 100` |
| `processed_at` | UTC timestamp of when this request was processed (ISO 8601) |

---

### `POST /api/profiles`

Create a demographic profile for a name. Calls Genderize, Agify, and Nationalize concurrently, then persists the result. Idempotent — returns the existing profile if one already exists for that name.

**Request body**

```json
{ "name": "james" }
```

**Success response (201)**

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
    "country_probability": 0.08,
    "created_at": "2026-04-15T10:00:00Z"
  }
}
```

| Field | Description |
|-------|-------------|
| `id` | UUID of the stored profile |
| `age_group` | `"child"` (≤12), `"teenager"` (13–19), `"adult"` (20–59), `"senior"` (60+) |
| `country_id` | ISO 3166-1 alpha-2 code of the most probable country |

---

### `GET /api/profiles`

List all profiles. Supports optional filtering.

**Query parameters (all optional)**

| Name | Description |
|------|-------------|
| `gender` | Filter by gender (case-insensitive) |
| `country_id` | Filter by country code (case-insensitive) |
| `age_group` | Filter by age group (case-insensitive) |

**Example request**

```
GET /api/profiles?gender=female&country_id=NG
```

**Success response (200)**

```json
{
  "status": "success",
  "count": 2,
  "data": [...]
}
```

---

### `GET /api/profiles/{id}`

Retrieve a single profile by UUID.

**Success response (200)** — same shape as `POST /api/profiles`.

**Error:** `404 Not Found` if no profile with that ID exists.

---

### `DELETE /api/profiles/{id}`

Delete a profile by UUID.

**Success response (204 No Content)**

**Error:** `404 Not Found` if no profile with that ID exists.

---

## Error responses

All errors share the same shape:

```json
{
  "status": "error",
  "message": "<description of what went wrong>"
}
```

| Scenario | Status code |
|----------|-------------|
| Missing or invalid request parameter | `400 Bad Request` |
| No prediction available for the name | `404 Not Found` |
| Profile not found | `404 Not Found` |
| External API unreachable | `502 Bad Gateway` |
| Database error | `500 Internal Server Error` |

---

## Running tests

```bash
go test ./...
```

Tests use `httptest` to mock the external API — no network or database required.

---

## Tech

- Language: Go 1.21+
- Database: PostgreSQL (via `pgx/v5`)
- External APIs: [Genderize.io](https://genderize.io), [Agify.io](https://agify.io), [Nationalize.io](https://nationalize.io)
