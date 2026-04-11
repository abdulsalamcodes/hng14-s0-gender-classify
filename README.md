# hng14-s0-gender-classify

A simple REST API that takes a name and tells you the likely gender — built for HNG Internship 14, Backend Track Stage 0.

It wraps the [Genderize.io](https://genderize.io) API and adds a confidence score, a UTC timestamp, and consistent error responses.

---

## Getting started

**Prerequisites:** Go 1.21+

```bash
git clone https://github.com/abdulsalamcodes/hng14-s0-gender-classify.git
cd hng14-s0-gender-classify
go run main.go
```

Server starts on port `8080`.

---

## Endpoint

### `GET /api/classify`

**Query parameter**

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
    "processed_at": "2026-04-11T10:00:00Z"
  }
}
```

| Field | Description |
|-------|-------------|
| `gender` | `"male"` or `"female"` |
| `probability` | How confident genderize.io is (0–1) |
| `sample_size` | Number of data points used for the prediction |
| `is_confident` | `true` only when `probability >= 0.7` AND `sample_size >= 100` |
| `processed_at` | UTC timestamp of when this request was processed (ISO 8601) |

**Error response**

All errors follow the same shape:

```json
{
  "status": "error",
  "message": "<description of what went wrong>"
}
```

| Scenario | Status code |
|----------|-------------|
| Missing or empty `name` | `400 Bad Request` |
| Genderize has no prediction for the name | `404 Not Found` |
| Genderize API unreachable or returns an error | `502 Bad Gateway` |

---

## Running tests

```bash
go test ./...
```

Tests use `httptest` to mock the external API — no network required.

---

## Tech

- Language: Go
- External API: [Genderize.io](https://genderize.io)
