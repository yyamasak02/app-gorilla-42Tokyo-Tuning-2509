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

## Backend Internal モジュール概要
- **`backend/internal/db`**
  - `backend/internal/db/db.go`: sqlx による接続プール初期化。`SetMaxOpenConns` や `SetConnMaxLifetime` の調整は接続枯渇対策の主なレバーになります。
- **`backend/internal/model`**
  - `backend/internal/model/model.go`: ユーザー／商品／注文／配送計画などの構造体を一元管理し、リポジトリ層で使用する DB タグを保持しています。
- **`backend/internal/repository`**
  - `backend/internal/repository/store.go`: 複数リポジトリを束ねてサービス層へ 1 つの依存として渡すアダプタ。
  - `backend/internal/repository/product.go`: 商品一覧と件数取得。1 リクエストで 2 回 SELECT するため `name/value/weight` のインデックスと短期キャッシュを検討。
  - `backend/internal/repository/order.go`: 注文 CRUD と一括ステータス更新。配送計画向けに `products` と JOIN します。
  - `backend/internal/repository/session.go`: cookie ベースのセッション登録・照会。
- **`backend/internal/service`**
  - `backend/internal/service/auth.go`: パスワード認証とセッション発行を担当。
  - `backend/internal/service/product.go`: 商品一覧取得 (`ProbeProducts` による計測付き)、注文件数分の一括登録、配送計画キャッシュのフラッシュ。
  - `backend/internal/service/robot.go`: 商品単位での指数分割＋ナップサック計算により配送計画を生成。キャッシュヒット時のショートカットとトレーシングを実装。
  - `backend/internal/service/order.go`: 注文履歴の取得をリポジトリに委譲しつつ、タイムアウト制御を付与。
- **`backend/internal/handler`**
  - `backend/internal/handler/auth.go`: `/api/login` を処理し、`session_id` Cookie を発行。
  - `backend/internal/handler/product.go`: 商品一覧・注文作成 API、画像ファイルを読み込んで返す `GetImage` を提供。
  - `backend/internal/handler/order.go`: 注文履歴 API。
  - `backend/internal/handler/robot.go`: ロボット向け配送計画・ステータス更新 API。
- **`backend/internal/middleware`**
  - `backend/internal/middleware/auth.go`: セッション Cookie 認証とロボット用 API キーチェック。
  - `backend/internal/middleware/jaeger.go` / `tracing.go`: Jaeger/OTEL トレース初期化の補助関数。
- **`backend/internal/server`**
  - `backend/internal/server/server.go`: DI（リポジトリ→サービス→ハンドラ）とルーティング、otelchi の組み込み、HTTP 起動処理。
- **`backend/internal/telemetry`**
  - `backend/internal/telemetry/telemetry.go`: 環境変数から Jaeger/OTLP エクスポータを自動設定し、トレーサーを提供。

### 運用改善アイデア（nginx などで任せられるもの）
- `backend/internal/handler/product.go:70` の画像配信は Go で毎回ファイル読み出しと Content-Type 判定をしている。`nginx` から `/images/*` を直接返すようにすればバックエンド負荷を低減できる。
- `/api/v1/product` は毎回一覧＋件数の 2 クエリを発行するため、Go 側で短期キャッシュ or ETag を付ければ `nginx` のキャッシュや CDN を活用しやすい。
- セッション Cookie の保護は `middleware/auth.go` で実施しているが、TLS 終端を担う `nginx` 側で HSTS・`Secure`/`SameSite` 設定を強化すると安全性が高まる。
- 配送計画 API (`/api/robot/delivery-plan`) は CPU 集約処理なので、Go で処理する前に `nginx` のレートリミットで突発的なアクセスを吸収できる。

### TODOリスト
  1. 商品一覧 API のキャッシュ導入
      - 検索条件・ページ・ソートに基づく短期キャッシュ（go-cache 等）をサービス層に挟み、同じリクエ
  ストが続く際の DB アクセスを削減する。
  2. orders テーブルへの product_weight / product_value 追加
      - 注文作成時に products の重量・価値をコピーして保持し、配送計画や注文履歴での products JOIN
  を減らす。
  3. products テーブルのインデックス強化
      - name, value, weight で必要な複合インデックスを再確認・追加し、フルスキャンを避ける。
  4. CountProducts のキャッシュ化
      - 一覧と件数がセットで動くので、件数を短時間キャッシュし、毎回の COUNT(*) 負荷を軽減する。
    
    -> mysql.confの更新と同時にキャッシュ機能を追加したら遅くなった。
      -> TODO：mysql.confを消してキャッシュ機能だけの時の速度を確認
        -> status: In Progress(1)
      -> TODO：mysql.confだけの時の速度を確認
        -> status: In Progress(2)
    -> ordersテーブルにweightとか追加
      -> status: In Progress(3)

### VM環境情報
azureuser@ftt2508-app-gorilla:~/app-gorilla-42Tokyo-Tuning-2509$ cat /proc/cpuinfo
processor       : 0
vendor_id       : AuthenticAMD
cpu family      : 25
model           : 1
model name      : AMD EPYC 7763 64-Core Processor
stepping        : 1
microcode       : 0xffffffff
cpu MHz         : 2445.426
cache size      : 512 KB
physical id     : 0
siblings        : 2
core id         : 0
cpu cores       : 1
apicid          : 0
initial apicid  : 0
fpu             : yes
fpu_exception   : yes
cpuid level     : 13
wp              : yes
flags           : fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush mmx fxsr sse sse2 ht syscall nx mmxext fxsr_opt pdpe1gb rdtscp lm rep_good nopl cpuid extd_apicid tsc_known_freq pni pclmulqdq ssse3 fma cx16 sse4_1 sse4_2 movbe popcnt aes xsave avx f16c rdrand hypervisor lahf_lm cmp_legacy cr8_legacy abm sse4a misalignsse 3dnowprefetch osvw topoext vmmcall fsgsbase bmi1 avx2 smep bmi2 erms rdseed adx smap clflushopt clwb sha_ni xsaveopt xsavec xgetbv1 xsaves xsaveerptr arat umip vaes vpclmulqdq rdpid fsrm
bugs            : sysret_ss_attrs null_seg spectre_v1 spectre_v2 spec_store_bypass srso
bogomips        : 4890.85
TLB size        : 2560 4K pages
clflush size    : 64
cache_alignment : 64
address sizes   : 48 bits physical, 48 bits virtual
power management:

processor       : 1
vendor_id       : AuthenticAMD
cpu family      : 25
model           : 1
model name      : AMD EPYC 7763 64-Core Processor
stepping        : 1
microcode       : 0xffffffff
cpu MHz         : 2445.426
cache size      : 512 KB
physical id     : 0
siblings        : 2
core id         : 0
cpu cores       : 1
apicid          : 1
initial apicid  : 1
fpu             : yes
fpu_exception   : yes
cpuid level     : 13
wp              : yes
flags           : fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush mmx fxsr sse sse2 ht syscall nx mmxext fxsr_opt pdpe1gb rdtscp lm rep_good nopl cpuid extd_apicid tsc_known_freq pni pclmulqdq ssse3 fma cx16 sse4_1 sse4_2 movbe popcnt aes xsave avx f16c rdrand hypervisor lahf_lm cmp_legacy cr8_legacy abm sse4a misalignsse 3dnowprefetch osvw topoext vmmcall fsgsbase bmi1 avx2 smep bmi2 erms rdseed adx smap clflushopt clwb sha_ni xsaveopt xsavec xgetbv1 xsaves xsaveerptr arat umip vaes vpclmulqdq rdpid fsrm
bugs            : sysret_ss_attrs null_seg spectre_v1 spectre_v2 spec_store_bypass srso
bogomips        : 4916.43
TLB size        : 2560 4K pages
clflush size    : 64
cache_alignment : 64
address sizes   : 48 bits physical, 48 bits virtual
power management:

azureuser@ftt2508-app-gorilla:~/app-gorilla-42Tokyo-Tuning-2509$ cat /proc/meminfo
MemTotal:        8134684 kB
MemFree:          990464 kB
MemAvailable:    4249092 kB
Buffers:          154992 kB
Cached:          3246068 kB
SwapCached:            0 kB
Active:          4432828 kB
Inactive:        2297180 kB
Active(anon):    2947180 kB
Inactive(anon):   395376 kB
Active(file):    1485648 kB
Inactive(file):  1901804 kB
Unevictable:       30724 kB
Mlocked:           27652 kB
SwapTotal:             0 kB
SwapFree:              0 kB
Zswap:                 0 kB
Zswapped:              0 kB
Dirty:                60 kB
Writeback:             0 kB
AnonPages:       3273352 kB
Mapped:           411548 kB
Shmem:              4552 kB
KReclaimable:     184804 kB
Slab:             274640 kB
SReclaimable:     184804 kB
SUnreclaim:        89836 kB
KernelStack:        8320 kB
PageTables:        37316 kB
SecPageTables:         0 kB
NFS_Unstable:          0 kB
Bounce:                0 kB
WritebackTmp:          0 kB
CommitLimit:     4067340 kB
Committed_AS:    7187708 kB
VmallocTotal:   34359738367 kB
VmallocUsed:       38400 kB
VmallocChunk:          0 kB
Percpu:             1800 kB
HardwareCorrupted:     0 kB
AnonHugePages:   2025472 kB
ShmemHugePages:        0 kB
ShmemPmdMapped:        0 kB
FileHugePages:         0 kB
FilePmdMapped:         0 kB
Unaccepted:            0 kB
HugePages_Total:       0
HugePages_Free:        0
HugePages_Rsvd:        0
HugePages_Surp:        0
Hugepagesize:       2048 kB
Hugetlb:               0 kB
DirectMap4k:      111928 kB
DirectMap2M:     7227392 kB
DirectMap1G:     3145728 kB
azureuser@ftt2508-app-gorilla:~/app-gorilla-42Tokyo-Tuning-2509$ 
