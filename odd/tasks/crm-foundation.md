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
- **Online only in v1 (user decision, 2026-11-08):** all clients call the API directly, with no local SQLite or sync endpoints. Business tables keep `updated_at` and soft delete so sync can be added later.
- **Local ports:** must not clash with `contacts` (Postgres 5433) or `planner` (Postgres 5434, API 8080). Use Postgres 5435 and API 8085.
- Generated artifacts, UI copy included, are in English.

## Delivery

- Forecast: about 8,000 to 10,000 authored changed lines, above the 400-line delivery budget.
- Strategy: `ask-on-risk`. Chain strategy: `stacked-to-main` (user decision, 2026-11-08).
- Slice boundaries: one pull request per task, T1 through T12, in order. Commits per pull request are recorded in Progress.
- Reviewed boundary: `8db8e0a`, the empty root commit of `main`.
- Push and pull request creation remain the user's decision.

## Checklist

Route for every task: delegated direct, because each touches 2+ non-trivial files (writer trigger).

- [ ] **T1 Backend skeleton.** Go module, config, migrations runner, server entry point, health route, Compose, `dev.sh`, `.env.example`, README. Accept: server boots against Compose Postgres, applies migrations, `GET /health` returns 200. Checks: `go build ./...`, `go vet ./...`, `go test ./...`.
- [ ] **T2 Auth.** Users, bcrypt, access JWT, refresh token rotation with family revocation. Accept: full auth flow and reuse detection covered by tests. Checks: `go test ./...`.
- [ ] **T3 Workspaces and tenancy.** Workspaces, members, roles, membership middleware. Accept: tests prove a non-member gets 404 and role limits hold. Checks: `go test ./...`.
- [ ] **T4 Contacts and companies.** CRUD, search, pagination, soft delete, contact-to-company link. Accept: cross-workspace isolation tests pass. Checks: `go test ./...`.
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

## Next step

T1 in progress through one delegated writer.
