# BeerOklog Backend

Go API for BeerOklog: SSO auth, shared beer catalog, per-user pours/reviews, public feeds, and S3 photo storage.

Mobile apps and the web talk to this service over HTTP only.

## Quick start

```bash
cp .env.example .env   # optional; compose already sets defaults
docker compose up
```

| Service | URL |
|---------|-----|
| API | http://localhost:8082 |
| MinIO console | http://localhost:9001 |
| Postgres | localhost:5432 |

Health check: `GET /healthz`

Without Docker: start Postgres + MinIO, copy `.env.example` → `.env`, then:

```bash
go run ./cmd/server
```

Mint a local JWT for API testing:

```bash
go run ./cmd/devtoken -user <user-uuid>
```

## Domain model (read this first)

| Concept | Table / API | Meaning |
|---------|-------------|---------|
| **Beer** | `beers` | Global catalog master data (shared) |
| **Review / pour** | `reviews` | Per-user tasting entry linked to a beer |
| **Log-only** | `rating = 0` and empty `review_text` | Private journal entry |
| **Published** | `rating > 0` **or** non-empty notes | Appears on public feeds |

Rules enforced in handlers:

1. Create review with `beer_id` **or** inline `beer` → find-or-create by case-insensitive name+brewery.
2. Update review **does not** mutate the beer catalog (older clients may still send `beer`; it is ignored).
3. Public/auth feeds filter to published reviews only.
4. Beer `avg_rating` / `review_count` ignore `rating = 0`.
5. `GET /api/reviews` returns the signed-in user’s full journal (including log-only).

## Admin catalog

Admins (`users.is_admin`) can edit beer master data and merge duplicates.

Grant admin by setting `ADMIN_EMAILS=you@example.com` (applied on sign-in/`/me`)
or:

```sql
UPDATE users SET is_admin = true WHERE email = 'you@example.com';
```

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/admin/beers/duplicates` | Exact name+brewery duplicate groups |
| POST | `/api/admin/beers/merge` | Manual merge (`keep_id`, `merge_ids`) |
| POST | `/api/admin/beers/dedupe-exact` | Auto-merge all exact groups |
| PATCH | `/api/admin/beers/{id}` | Edit catalog fields |

Prevention: `POST /api/beers` and review create both find-or-create by case-insensitive name+brewery so identical info reuses a row.

Web UI: `/admin/catalog`.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for diagrams, routes, conventions, and testing.

## Stack

- Go 1.25, Chi v5, pgx v5, PostgreSQL 16
- JWT access + refresh (Google / Apple SSO)
- S3-compatible storage (MinIO locally)
- Embedded SQL migrations on startup

## Docs in this repo

| Doc | Contents |
|-----|----------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Architecture, API map, domain rules, conventions |
| `.env.example` | Environment variables |
| `BRANCH_NOTE.md` | Branch notes (if present) |

Cross-platform product context lives in the parent monorepo: `../docs/TECH_SPEC.md`.
