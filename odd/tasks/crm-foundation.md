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
- [x] **T5 Pipelines and deals.** Pipelines, ordered stages, deals, stage moves; a default pipeline on workspace creation. Accept: isolation and move tests pass. Checks: `go test ./...`.
- [x] **T6 Activities.** Activities linked to contact, company, or deal, with due and completed state. Accept: isolation and link validation tests pass. Checks: `go test ./...`.
- [x] **T7 Web shell and auth.** SvelteKit scaffold, API client with refresh, login and register, workspace switcher. Accept: login reaches the app shell in a browser. Checks: `pnpm check`, `pnpm build`.
- [x] **T8 Web contacts and companies.** List, search, create, edit, delete. Checks: `pnpm check`, `pnpm build`, browser run.
- [x] **T9 Web pipeline and activities.** Kanban board with stage moves, deal detail, activity timeline. Checks: `pnpm check`, `pnpm build`, browser run.
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

| T5 | `bce3adf`, `7df945d`, `31e2d68`, `3dc98bc` | build, vet, gofmt clean; 783 test passes as `crm_app`, parent re-ran `go test`; default pipeline, deal moves, and cross-workspace 404 observed over HTTP | medium; one review of the whole range exceeded the reviewer context budget, so each commit was reviewed separately with owner consent: 4 of 4 approved and acknowledged |

| T5 follow-ups and T6 | `743a361`, `62c2286`, `b00fd92`, `9e941bd`, `ec47be6` | build, vet, gofmt clean; 928 test passes as `crm_app`, parent re-ran `go test`; timeline, task completion, unlink on delete, 409 on last pipeline, and cross-workspace 404 observed over HTTP | medium, granted, one review of the range approved and acknowledged |

| T6 fixes and T7 | `75a2724`, `d923bc9`, `b58e513` | backend 950 test passes; web `pnpm check` clean, 59 Vitest passes, `pnpm build` ok; parent re-ran web tests and backend `app` and `store`; register, workspaces, reload, logout, and phone width observed in a browser | high, granted, four lenses approved and acknowledged |

| T7 fixes and T8 | `4539005`, `98c0fbf`, `f5ab0fd`, `19978d5`, `ae5665f` | web `pnpm check` clean, 114 Vitest passes, `pnpm build` ok; parent re-ran web tests; create, edit (clear, unlink, relink), paging, search in the URL, delete dialog, not found, workspace switch, phone width, and two tabs restoring together observed in a browser | medium; assess on the whole range said `under_budget` at 3,228 lines, so each commit was reviewed separately with owner consent: 4 of 4 approved and acknowledged, the fourth after one correction for a critical finding (clearing the company picker did not unlink before submit, fixed in `ae5665f`) |

The three T7 warnings are fixed in `4539005`: tabs refresh in turn under a Web Lock and read the latest stored token once they hold it (browsers without Web Locks rely on `REFRESH_GRACE`); signing out clears the restore error; one shared no-storage fallback in `web/src/lib/storage.js`.

T8 decisions: a company's people are found by searching contacts by the company name and filtering on `company_id` on the client, because `GET /contacts` has no `company_id` filter (a backend filter would replace this); paging uses "Load more"; search is written to `?q=` after 300 ms, replacing the history entry; a company is linked only by choosing it in the picker and unlinked by clearing the field or the remove button; an edit with no changes sends no request; the confirm dialog uses the native `<dialog>`; "New contact" and "New company" are header buttons. `PRODUCT.md` was not written; the `contacts` design stood in for it.

| T9 | `0e9a483`, `3ffdac4`, `6471139`, `e42c2c0`, `2c3e4f8` | web `pnpm check` clean, 179 Vitest passes, `pnpm build` ok; parent re-ran web tests; board, drag and reorder, keyboard move, lost reason, rollback on a failed move, deal create, edit, detail, delete, not found, timeline for each kind, complete and reopen, phone width, and dark mode observed in a browser | medium; assess on the whole range said `under_budget` at 3,852 lines, so the commits were reviewed separately with owner consent: 4 reviews (the last covers `e42c2c0` and `2c3e4f8` together), all approved and acknowledged with no correction |

T9 decisions: all open deals load (200 per page, up to 25 pages, with a notice beyond that); won and lost columns show the first 50 deals with "Load more"; a move's position is the dense index among the loaded deals of the target column, and appending to a closed column with unloaded pages sends no position; a failed move puts the card back at its old stage and index (`undoMove`), keeps other moves, shows the API reason, and reloads on 404; a drag into a lost stage opens the move dialog with the place preset, asking only for the reason; the deal page stage control moves a deal to the end of the stage; currency defaults to USD and the value is typed in the main unit and converted by the currency's decimals; one `RecordPicker` serves companies and contacts; edit never sends the pipeline or stage; the timeline shows only activities linked to that record, 50 per page; `/pipeline` widens `main` to 1680px; an unknown `?pipeline=` opens the first pipeline.

T9 review warnings, not yet fixed:

- Parsing a local due time and a PATCH with an invalid due or cleared value (`web/src/lib/activities/fields.js:35-40`, `:84-86`; `web/src/lib/deals/fields.js:97-98`).
- The move flow is untested, and the position select is disabled in the dialog (`web/src/lib/deals/board.svelte.js:150-165`; `web/src/lib/components/MoveDealDialog.svelte:80-85`).
- Clearing the owner on edit, and a stale stage select on the deal page (`web/src/lib/components/DealForm.svelte:44-46`; `web/src/routes/(app)/deals/[id]/+page.svelte:70-72`).
- A race between editing and cancelling an activity, and a stale "now" for due state (`web/src/lib/components/ActivityTimeline.svelte:149`, `:38`).
- No component tests for the deal form or the timeline.

T9 follow-ups not built: creating, renaming, reordering, and deleting pipelines and stages; deal filters beyond the pipeline; reports; the dashboard still says the pipeline is coming next; the timeline composer is tall.

T8 review warnings, not yet fixed:

- A hung refresh holds the cross-tab lock, and a refused lock request is not mapped to an app error (`web/src/lib/api/client.js:124-126`).
- `web/src/lib/records/screens.svelte.js:83-108` and `:25-77`, and `web/src/lib/records/fields.js:139-142` (reliability warnings R3-001 to R3-003).
- A click on the confirm dialog's padding cancels it (`web/src/lib/components/ConfirmDialog.svelte:53-55`).
- An invalid date throws on the company page (`web/src/routes/(app)/companies/[id]/+page.svelte:188`), and the page can read a null company after an await (`:72-73`).
- A stale active option can be chosen with Enter in the picker (`web/src/lib/components/CompanyPicker.svelte:72-75`), and a failed company preset on a new contact is silent (`web/src/routes/(app)/contacts/new/+page.svelte:22-26`).
- The screens and the picker have no component tests.

The two T6 warnings are fixed: `lost_reason` is judged by the target stage under the pipeline lock, and `due_at` is checked at microsecond precision against the schema bounds.

T7 decisions: access token in memory and refresh token in `localStorage`; a refresh that fails for any reason other than a refused token keeps the session; Kit 2 and Vite 7, matching `contacts`; English only, with no i18n layer; the dev server is fixed to port 5173 for CORS; `VITE_API_URL` defaults to `http://localhost:8085`.

T7 review warnings, not yet fixed:

- Single-flight refresh covers one tab only, so tabs restoring together can reuse one refresh token and trigger family revocation (`web/src/lib/api/client.js:100-109`).
- After a failed restore, signing out leaves the restore error screen up (`web/src/lib/session.svelte.js:91-95`).
- Two divergent no-storage fallbacks (`web/src/lib/workspaces.svelte.js:11-18`).

T5 follow-ups are fixed: the last live pipeline returns 409, the blank `lost_reason` case, the README table, the two weak tests, and invalid UTF-8 on contacts and companies.

T6 decisions: `due_at`, `assignee_id`, and `completed_at` exist only on tasks, enforced by handlers and a check constraint; `kind` never changes; only the author, an owner, or an admin edits or deletes an activity, and every member reads, creates, completes, and reopens; deleting a linked record clears that link, and an activity left with no link is soft-deleted; removing a member clears author and assignee; the timeline is newest first.

T6 review warnings, not yet fixed:

- A blank or `null` `lost_reason` is refused on every move, including into a lost stage, and its message contradicts the lost-stage rule (`backend/pkg/app/deals.go:283-286`).
- A `due_at` with fractional seconds in the last second of 2200 passes the handler and fails the database check as a 500 (`backend/pkg/app/activities.go:52-59`).

T5 decisions: stages soft-delete with `position` NULL; dense integer positions serialized on the pipeline row lock, with last-wins moves and no version check; a removed member's deals keep no owner and do not regain one on rejoin; deleting a pipeline or stage with live deals returns 409; deals start only in an open stage; value is integer minor units; migration 009 refuses a role that cannot bypass Row-Level Security.

T5 review warnings, none blocking and not yet fixed:

- `DeletePipeline` can delete a workspace's last pipeline, leaving nowhere to create deals (`backend/pkg/store/pipelines.go:215-230`). Owner decision, 2026-11-09: return 409.
- A blank `lost_reason` sent to an open or won stage returns 200, while the README says 400 (`backend/pkg/app/deals.go:276`).
- A blank line splits the README route table, so new rows render as plain text (`README.md:106`).
- `TestConcurrentStageWritesKeepPositionsDense` does not race reorder or delete (`backend/pkg/store/pipelines_test.go:579-590`).
- `TestUnlinkingStaysInsideTheWorkspace` probably stays green without the `workspace_id` filter in `unlinkDeals` (`backend/pkg/store/deal_links_test.go:417-444`).
- Contacts and companies still accept invalid UTF-8 through `decodeJSON`, unlike pipelines and deals.

T4 decisions: soft-deleting a company clears its contacts' links; every member can read and write contacts and companies; a `PATCH` with an empty string clears an optional field, `null` on a text field returns 400, and `company_id: null` unlinks; a `POST` still treats a null optional field as blank; `limit` is 1 to 200 with no totals; search has no trigram index because `ILIKE` is not leakproof under Row-Level Security, so very large workspaces will need another design.

### Open items

- `.env.example` was created by the owner and lacks the optional `AUTH_SESSION_RATE_LIMIT_PER_MINUTE` (default 600). The agent's permission settings deny editing it.
- RLS risks from T3: it does not stop SQL injection, because arbitrary SQL can call `set_config`; it does not enforce soft delete; an owner or admin can add any registered email with no consent step.
- Pending owner decision: register returns 409 for a taken email, which reveals accounts (`backend/pkg/app/auth.go:101-104`). Hiding it needs email verification, which is outside this feature.
- Eight review suggestions are not scheduled.

## Next step

T4 was merged directly to `main` and pushed at `636075b` (owner decision, 2026-11-09). T5 was merged directly to `main` and pushed at `ef8c086` (owner decision, 2026-11-09). T6 was merged directly to `main` and pushed at `1f24dc4` (owner decision, 2026-11-09). The backend, T1 through T6, is complete. T7 was merged directly to `main` and pushed at `b103032` (owner decision, 2026-11-09). T8 was merged directly to `main` and pushed at `18f43d5` (owner decision, 2026-11-09; last reviewed boundary `ae5665f`). T9 is complete on `feat/crm-web-pipeline` and reviewed (last reviewed boundary `2c3e4f8`); delivery awaits the owner. Next: T10, the Expo shell and auth. Clients follow the visual design of `contacts` (owner decision, 2026-11-09): its `DESIGN.md`, `.impeccable/design.json`, and web components. `origin` uses SSH on port 443 because port 22 times out.
