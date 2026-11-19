# Delcom Blog — Backend REST API (Golang Gin)

Backend murni REST API (decoupled deployment + CORS) untuk studi kasus **Delcom Blog Multi-Tier & AI Generator dengan Integrasi Search Engine**. Berjalan pada **port 8081**.

Arsitektur **Feature-First + Clean Layering** (`dto → repository → service → handler`), **GORM** (PostgreSQL saat runtime, SQLite `:memory:` saat testing), JWT, Role Guard, Rate Limiter, dan Swagger dinamis.

## Struktur

```
cmd/server/main.go          entry point
docs/                       OpenAPI (api.json di-embed) + docs.go
internal/config             konfigurasi .env
internal/database           model, koneksi, migrasi, seeder idempoten
internal/middleware         CORS, Auth JWT, RequireRole, RateLimiter
internal/response           format JSON standar
internal/features/auth      register, login, logout, profil
internal/features/post      artikel publik, artikel penulis, statistik, admin
internal/features/ai        OpenAI client, search engine client, AI Blog Generator
internal/server/router.go   routing + Swagger + 404 handler
```

## Format Respons

```json
{ "status": "success|fail|error", "message": "...", "data": { } }
```
`data` hanya ada bila endpoint mengembalikan data. Route tak dikenal: `{"status":"fail","message":"Endpoint not found"}`.

## 21 Endpoint

| # | Method | Path | Akses |
|---|--------|------|-------|
| 1 | GET | `/swagger/*any` | Publik |
| 2 | GET | `/api.json` | Publik |
| 3 | GET | `/api/public/categories` | Publik |
| 4 | GET | `/api/public/posts?q=&category=` | Publik |
| 5 | GET | `/api/public/posts/:slug` | Publik |
| 6 | POST | `/api/auth/register` | Publik |
| 7 | POST | `/api/auth/login` | Publik |
| 8 | POST | `/api/auth/logout` | Auth |
| 9 | GET | `/api/auth/profile` | Auth |
| 10 | PUT | `/api/auth/profile` | Auth |
| 11 | GET | `/api/posts` | Auth |
| 12 | POST | `/api/posts` | Auth |
| 13 | GET | `/api/posts/stats` | Auth |
| 14 | GET | `/api/posts/:id` | Auth |
| 15 | PUT | `/api/posts/:id` | Auth |
| 16 | PATCH | `/api/posts/:id/toggle-publish` | Auth |
| 17 | DELETE | `/api/posts/:id` | Auth |
| 18 | POST | `/api/posts/ai-generate` | Auth |
| 19 | GET | `/api/admin/users` | Admin |
| 20 | GET | `/api/admin/posts` | Admin |
| 21 | GET | `/api/admin/users/:id/posts` | Admin |

Catatan perilaku:
- `reading_time = total kata / 200` (minimal 1). Slug dibuat otomatis dan unik (`judul`, `judul-2`, ...).
- Registrasi selalu menghasilkan role `member` (role tidak bisa dikirim klien).
- Artikel milik penulis lain dijawab `404` (keberadaannya tidak dibocorkan).
- AI Generator: `include_web_search: true` memanggil search engine; bila gagal, artikel tetap dibuat tanpa sumber. Respons: `title`, `category`, `excerpt`, `content` (Markdown + bagian `## Referensi`), `search_sources` (selalu array).

## Konfigurasi

Salin `.env.example` ke `.env` lalu sesuaikan (lihat file tersebut). Akun awal (seeder idempoten): `admin/admin` (dari `ADMIN_USERNAME`/`ADMIN_PASSWORD`) dan `member/member`.

## Menjalankan

```bash
go mod tidy
go run ./cmd/server          # http://localhost:8081  | Swagger: /swagger/index.html
```

## Pengujian

```bash
go test -v -cover ./internal/...
```
