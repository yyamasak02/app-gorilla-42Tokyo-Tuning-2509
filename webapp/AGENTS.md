# Repository Guidelines

## Project Structure & Module Organization
Backend Go service in `backend/` (handlers `internal/`, entry `cmd/`), Next.js UI in `frontend/` (`src/app`, shared UI `src/components`), Playwright e2e in `e2e/`. Ops assets: `mysql/`, `nginx/`, `images/`. Use `docker-compose.local.yml` for dev and `docker-compose.yml` for production-like runs on the shared `webapp-network`.

## Build, Test, and Development Commands
- `docker compose -f docker-compose.local.yml up --build`: boot full stack with hot reload.
- `go run ./cmd` (`backend/`): direct API run; set `DATABASE_URL`.
- `yarn dev --turbo` (`frontend/`): Next.js dev server.
- `yarn build`: frontend production bundle; pair with Docker production targets.
- `docker compose down -v`: reset containers and MySQL volumes.

## Coding Style & Naming Conventions
Run `gofmt`/`goimports` before committing; exported Go identifiers PascalCase, locals camelCase, services prefer interfaces. Frontend must pass `yarn lint`; components PascalCase, hooks camelCase, styles co-located or via MUI `sx`. TypeScript stays strict—declare types and skip `any`.

## Testing Guidelines
Add table-driven `*_test.go` near Go services and run `go test ./...`. Place UX journeys in `e2e/tests/feature-name.spec.ts` and execute with `cd e2e && yarn test` against a running stack. Cover success and failure paths for both backend and UI additions.

## Commit & Pull Request Guidelines
Keep the `[ADD]`/`[FIX]`/`[UPDATE]` prefix format and imperative subjects. PRs should summarise intent, link issues, flag DB or tracing changes, and include UI screenshots when visuals change. Run `yarn lint`, `go test ./...`, and relevant Playwright suites before review; call out skips.

## Environment & Configuration Tips
Tracing is on by default (`TRACE_ENABLED`, `JAEGER_ENDPOINT`); keep compose files aligned. Do not commit overrides for MySQL creds or `e2e/tokens/`. Rotate TLS assets by updating `images/` and `nginx/` together.

## Fix Follow-up
**Load Test Hotspots**
- Catalog flow (rules Step4-1) pounds `/api/v1/product`; add indexes on `products(name)`, `products(value)`, `products(weight)` plus a covering `(name, product_id)` index to avoid scans.
- Order history (Step4-1.7〜1.9) needs `(user_id, created_at)` and `(user_id, product_id, shipped_status)` indexes so pagination, search, and status filters stay in index.
- Catalog totals issue duplicate `COUNT(*)`; cache totals per filter or maintain a counter table to trim response time.
- Delivery planning (Step4-2) benefits from `(shipped_status, product_id)` and fetching only weights/values to shrink the knapsack input.

**Delivery Plan Cache**
- Implemented `DeliveryPlanCache` (5s TTL, key = robot + capacity) to skip redundant plan generation during load.
- Cache flushes on order creation and status updates to keep shipping workloads fresh.

## Benchmark & Test Notes
- ベンチ指標 `bench_robot_getDeliveryPlan_fail_count` は `/api/robot/delivery-plan` のタイムアウト・5xx に起因。shipping 行が10万件超あるため、同一商品の明細をそのままナップサックへ投入すると指数的に処理が膨れ上がり失敗が 33,888 件まで増えた。
- 最適化方針は「重量が容量を超えるレコードは取得時に除外」「商品ごとに ID 昇順で束ね、容量分だけ 1,2,4,... と指数分割して knapsack item 化」することで、計算量を桁違いに削減しつつ最適解を保つ。
- E2E テストは Playwright で `workers:1 / timeout:600s / expect timeout:120s` 設定。`orders.test.ts` では配送計画 API とサンプルJSONを完全一致比較、`robot.e2e.ts` はログイン→注文→配送計画→ステータス更新の一連フロー、`product.test.ts` は 100万件規模の検索応答・UI反映を検証する。
- テスト実行時間は1ケースあたり30〜90秒程度で、配送計画APIのレスポンスが遅いと待ち時間がそのまま失敗要因になる。
