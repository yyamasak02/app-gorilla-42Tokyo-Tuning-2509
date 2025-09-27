# パフォーマンス最適化メモ

## 初期値 -> score 224　

## データベース接続設定の変更　-> score +20

### 変更内容
- **MaxOpenConns**: 25 → 100（同時接続数増加）
- **MaxIdleConns**: 10 → 50（アイドル接続増加）
- **ConnMaxLifetime**: 0 → 5分（接続ライフタイム設定）

### 目的
K6の負荷テストで `robotAPIScenario` が1回しか実行されない問題を解決するため、データベース接続の最適化を実施。

## GetShippingOrders関数の修正 -> score ほぼ変わらず。

### 変更内容
- **WHERE条件**: `shipped_status = 'shipping'` → `shipped_status IN ('shipping', 'delivering')` → **元に戻す**
- **ORDER BY**: `created_at ASC` を追加（古い注文から優先処理）
- **LIMIT**: 100件制限を追加（メモリ使用量制御）

### 目的
robotAPIScenarioが継続的に実行できるよう、deliveringステータスの注文も処理対象に含める。→ **元の仕様に戻す**

## ナップサック問題のアルゴリズム最適化 -> score +150

### 変更内容
- **アルゴリズム**: O(2^n) 全探索 → O(n×capacity) 動的プログラミング
- **計算時間**: 120秒（タイムアウト） → 0.01秒
- **メモリ使用**: 再帰スタック → 2次元配列テーブル

### 目的
robotAPIScenarioが30秒の制限時間内に完了するよう、配送計画計算を高速化。