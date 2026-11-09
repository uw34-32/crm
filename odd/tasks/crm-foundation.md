# CRM foundation

Locator: `odd/tasks/crm-foundation.md`
Engram mirror: `odd/crm-foundation/tasks`

## Objective

Build a team CRM that follows the layered architecture of the `contacts` project.

## Problem and why

The owner needs a CRM for a team that shares customer data. `contacts` cannot serve as its source of truth today: every contact belongs to one `user_id`, auth is user JWT only (no API keys or scopes), there are no webhooks or batch reads, and the same API exposes bank accounts and identity cards.

## Scope

- Accounts and auth: register, login, refresh rotation, logout, current user.
- Workspaces with members and roles (`owner`, `admin`, `member`).
- Contacts and companies owned by a workspace.
- Pipelines, stages, and deals.
- Activities (notes, calls, meetings, tasks) linked to contacts, companies, or deals.
- Three client applications for all of the above, on one Go API (user decision, 2026-11-08): SvelteKit web, React Native (Expo), and Tauri desktop.

Out of scope for this feature: integration with `contacts`, email or calendar sync, billing and plans, imports, reports.

## Constraints and decisions

- **Standalone (user decision, 2026-11-08):** the CRM owns its contacts and companies. A nullable `external_contact_id` column keeps a later `contacts` integration possible; nothing reads it yet.
- **Architecture:** mirror `contacts`: `backend/cmd/server`, `backend/pkg/{app,auth,config,migrate,model,server,store}`, embedded SQL migrations, `web/` with SvelteKit 5, Tailwind 4, and adapter-static, `native-client/` with Expo and React Native, and Tauri in `native-client/src-tauri`. This is layered, not hexagonal: `App` depends on the concrete `*store.Store`.
- **Stack:** Go, Chi v5, pgx v5, golang-jwt v5, bcrypt, PostgreSQL 16 through Docker Compose.
- **Tenancy:** every business table has `workspace_id`; every store method takes it; a membership middleware resolves the workspace from the route. A resource in another workspace returns 404, never 403, so existence does not leak.
- **Row-Level Security as a second layer (user decision, 2026-11-08):** the `workspace_id` store filter stays the primary control. T2 splits database roles: a migration owner, and an app role that is neither superuser nor `BYPASSRLS`, with both flags checked at startup. T3 adds policies with `FORCE`, explicit `WITH CHECK`, a transaction-local `app.workspace_id` that fails closed, and composite foreign keys such as `(workspace_id, contact_id)`, because foreign key checks bypass RLS. Isolation tests connect as the app role. `dianisport-facturacion` is a list of mistakes to avoid here, not a template.
- **Online only in v1 (user decision, 2026-11-08):** all clients call the API directly, with no local SQLite or sync endpoints. Business tables keep `updated_at` and soft delete so sync can be added later.
- **Local ports:** must not clash with `contacts` (Postgres 5433) or `planner` (Postgres 5434, API 8080). Use Postgres 5435 and API 8085.
- Generated artifacts, UI copy included, are in English.

## Delivery

- Forecast: about 8,000 to 10,000 authored changed lines, above the 400-line delivery budget.
- Strategy: `ask-on-risk`. Chain strategy: `stacked-to-main` (user decision, 2026-11-08).
- T1 through T3 override (user decision, 2026-11-08): rebase on `origin/main`, merge directly to `main`, and push, with no pull requests. Later tasks keep the chain below unless the owner says otherwise.
- Slice boundaries: one pull request per task, T4 through T12, in order. Commits per pull request are recorded in Progress.
- Base: `44e1050`, the initial commit of `origin/main`, after rebasing.
- Push and pull request creation remain the user's decision.

## Checklist

Route for every task: delegated direct, because each touches 2+ non-trivial files (writer trigger).

- [x] **T1 Backend skeleton.** Go module, config, migrations runner, server entry point, health route, Compose, `dev.sh`, `.env.example`, README. Accept: server boots against Compose Postgres, applies migrations, `GET /health` returns 200. Checks: `go build ./...`, `go vet ./...`, `go test ./...`.
- [x] **T2 Auth.** Users, bcrypt, access JWT, refresh token rotation with family revocation. Accept: full auth flow and reuse detection covered by tests. Checks: `go test ./...`.
- [x] **T3 Workspaces and tenancy.** Workspaces, members, roles, membership middleware. Accept: tests prove a non-member gets 404 and role limits hold. Checks: `go test ./...`.
- [x] **T4 Contacts and companies.** CRUD, search, pagination, soft delete, contact-to-company link. Accept: cross-workspace isolation tests pass. Checks: `go test ./...`.
- [ ] **T5 Pipelines and deals.** Pipelines, ordered stages, deals, stage moves; a default pipeline on workspace creation. Accept: isolation and move tests pass. Checks: `go test ./...`.
- [ ] **T6 Activities.** Activities linked to contact, company, or deal, with due and completed state. Accept: isolation and link validation tests pass. Checks: `go test ./...`.
- [ ] **T7 Web shell and auth.** SvelteKit scaffold, API client with refresh, login and register, workspace switcher. Accept: login reaches the app shell in a browser. Checks: `pnpm check`, `pnpm build`.
- [ ] **T8 Web contacts and companies.** List, search, create, edit, delete. Checks: `pnpm check`, `pnpm build`, browser run.
- [ ] **T9 Web pipeline and activities.** Kanban board with stage moves, deal detail, activity timeline. Checks: `pnpm check`, `pnpm build`, browser run.
- [ ] **T10 Native shell and auth.** Expo and React Native scaffold, API client with refresh, secure token storage, login and register, workspace switcher. Checks: typecheck, unit tests, app run.
- [ ] **T11 Native CRM screens.** Contacts, companies, pipeline, deals, and activities. Checks: typecheck, unit tests, app run.
- [ ] **T12 Tauri desktop.** Tauri app in `native-client/src-tauri` wrapping the Expo web export, as in `contacts`. Accept: desktop build launches and logs in. Checks: Tauri build, app run.

## Progress

| Task | Commit | Verification | Review tier and outcome |
| --- | --- | --- | --- |
| T1 (partial, open: `.env.example`) | `71efd8f`, `a6014ec`, `1201a87` | build, vet, gofmt clean; 14/14 tests with database, parent re-ran `go test`; `/health` 200 and 503 observed; migrations idempotent | high, granted, approved and acknowledged |

| T1 closure and T2 | `bc6f259`, `96c6e30`, `112532d`, `ec3eacb`, `dce66bc`, `ab8b7a4`, `5231ba4` | build, vet, gofmt clean; 169 test passes as `crm_app`, parent re-ran `go test`; auth flow, reuse revocation, CORS, and rate limit observed over HTTP | high, granted, approved and acknowledged |

| Auth hardening and T3 | `1ebd9c1`, `e68532a`, `7df96db`, `9d626bc` | build, vet, gofmt clean; 339 test passes as `crm_app`, parent re-ran `go test`; workspace, role, and last-owner flows observed over HTTP | high, review declined by the owner for this candidate; independent verifier passed |

| T4 | `0a7f57a`, `328803a`, `75ef363`, `847bfd6` | build, vet, gofmt clean; 510 test passes as `crm_app`, parent re-ran `go test`; CRUD, search, and cross-workspace 404 observed over HTTP | medium, granted; one correction for a critical `PATCH` null finding; validated, approved and acknowledged |

`main` was pushed at `5eee932` on 2026-11-09. The T3 range has no review receipt, because the owner declined it. Last reviewed boundary: `847bfd6`.

T4 decisions: soft-deleting a company clears its contacts' links; every member can read and write contacts and companies; a `PATCH` with an empty string clears an optional field, `null` on a text field returns 400, and `company_id: null` unlinks; a `POST` still treats a null optional field as blank; `limit` is 1 to 200 with no totals; search has no trigram index because `ILIKE` is not leakproof under Row-Level Security, so very large workspaces will need another design.

### Open items

- `.env.example` was created by the owner and lacks the optional `AUTH_SESSION_RATE_LIMIT_PER_MINUTE` (default 600). The agent's permission settings deny editing it.
- RLS risks from T3: it does not stop SQL injection, because arbitrary SQL can call `set_config`; it does not enforce soft delete; an owner or admin can add any registered email with no consent step.
- Pending owner decision: register returns 409 for a taken email, which reveals accounts (`backend/pkg/app/auth.go:101-104`). Hiding it needs email verification, which is outside this feature.
- Eight review suggestions are not scheduled.

## Next step

Deliver T4 as the owner decides, then start T5. `origin` uses SSH on port 443 because port 22 times out.
