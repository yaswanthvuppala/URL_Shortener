# Production URL Shortener API (Go Standard Library)

A high-performance, robust URL Shortener API built using **Pure Go Standard Libraries** (`net/http`, `database/sql`, `crypto/rand`, `embed`), persistent storage (**SQLite** / **PostgreSQL**), and resilient caching (**Redis**).

This service maps long URLs to short, unique base62 codes, serves ultra-low-latency redirects, and tracks click analytics with atomic concurrency safety.

---

## Architecture & Design Highlights

- **Pure Go Standard Library Router (`net/http`)**:
  - Leverages Go 1.22+ native routing enhancements in `http.ServeMux` (`GET /`, `POST /shorten`, `GET /{short_code}`, `GET /analytics/{short_code}`).
  - Zero external routing frameworks (Chi, Gin, Echo removed) for minimal footprint and maximum longevity.
- **Embedded Static Assets (`embed.FS`)**:
  - The single-page frontend web UI (`static/index.html`) is compiled directly into the binary at build time. The resulting server executable is completely standalone.
- **Database Layer (`database/sql`)**:
  - Direct parameterized SQL queries without bulky ORMs.
  - Native atomic click increment query (`UPDATE url_mappings SET clicks = clicks + 1 WHERE short_code = ?`) eliminating race conditions.
  - Automatic table and index creation on startup.
  - Drivers:
    - **SQLite**: `modernc.org/sqlite` (Pure Go, 100% CGO-free, cross-compiles anywhere without GCC/MinGW).
    - **PostgreSQL**: `github.com/lib/pq` for production deployments.
- **Resilient Caching (`github.com/redis/go-redis/v9`)**:
  - Write-through cache on shorten with 24-hour TTL.
  - Cache-aside on redirect for instant redirection without database overhead.
  - **Graceful Degradation**: If Redis is unreachable, offline, or restarts, the service continues to operate seamlessly by falling back to the relational database with zero dropped requests.
- **Cryptographically Secure Short Codes (`crypto/rand`)**:
  - Uses `crypto/rand` with rejection sampling across a 62-character alphanumeric set (`[a-zA-Z0-9]`), preventing pseudo-random predictability and modulo bias.

---

## Database Schema (`url_mappings`)

| Column Name | Type | Constraints | Description |
| :--- | :--- | :--- | :--- |
| `id` | Integer / Serial | Primary Key | Unique autoincrement ID |
| `short_code`| String(10) | Unique, Index | 6-character unique base62 code |
| `long_url` | Text | Not Null | Target destination URL |
| `clicks` | Integer | Default: 0 | Total redirection click count |
| `created_at`| Timestamp | Default: CURRENT_TIMESTAMP | Record creation timestamp (UTC) |

---

## Prerequisites

- **Go 1.22+**
- **Docker & Docker Compose** (optional, for Redis or containerized execution)

---

## Quickstart

### 1. Build and Run Locally

```powershell
# Run the Go server directly (starts on port 8000 by default)
go run ./cmd/server
```

Or build a standalone binary:

```powershell
go build -o bin/server.exe ./cmd/server
./bin/server.exe
```

The server starts on `http://localhost:8000`.

### 2. Run with Docker Compose

Starts both Redis and the Go web service:

```bash
docker compose up --build -d
```

### 3. Environment Variables

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | Listening HTTP port | `8000` |
| `BASE_URL` | Base prefix for generated short URLs | `http://localhost:8000` |
| `DATABASE_URL`| SQLite or PostgreSQL DSN | `sqlite:///./url_shortener.db` |
| `REDIS_URL` | Redis connection URL | `redis://localhost:6379` |

*(Supports optional `.env` file loaded automatically on startup)*

---

## API Endpoints

### 1. Web UI
- **Endpoint**: `GET /`
- **Description**: Serves the embedded HTML/CSS/JS frontend application.

### 2. Shorten URL
- **Endpoint**: `POST /shorten`
- **Request Body**:
  ```json
  {
    "long_url": "https://go.dev"
  }
  ```
- **Response** (`201 Created`):
  ```json
  {
    "short_url": "http://localhost:8000/aB3xD2"
  }
  ```

### 3. Redirection
- **Endpoint**: `GET /{short_code}`
- **Response**: `307 Temporary Redirect` to the original long URL with automatic click increment.

### 4. Click Analytics
- **Endpoint**: `GET /analytics/{short_code}`
- **Response** (`200 OK`):
  ```json
  {
    "short_code": "aB3xD2",
    "long_url": "https://go.dev",
    "clicks": 42,
    "created_at": "2026-11-13T16:20:00Z"
  }
  ```

### 5. Health Check
- **Endpoint**: `GET /health`
- **Response** (`200 OK`):
  ```json
  {
    "status": "healthy"
  }
  ```

---

## Running Automated Tests

Run the test suite across all packages:

```powershell
go test -v ./...
```
