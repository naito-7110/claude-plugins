# infrastructure-terraform: Terraform 運用の具体

[infrastructure](../adr/infrastructure.md) プリセットの原則を Terraform で実現する手法集(非規範)。

## state 管理

- state はリモートバックエンドに置き、ロックを有効にする(ローカル state・ロックなし運用の禁止)
- **state に秘匿情報を残さない**: secrets はマネージド保管庫に置き、Terraform からはデータソース参照で解決する。state ファイルへのアクセス自体を秘匿情報アクセスとして扱い、最小権限にする

## plan / apply の流れ(昇格ゲートとの接続)

- PR で `terraform plan` を CI 実行し、差分を PR 上でレビュー可能にする(plan 結果がレビューの対象)
- `terraform apply` は昇格ゲート(マージ後・タグ push 等)の後にのみ実行する。手元からの apply を禁止する
- 環境ごとにディレクトリまたは workspace を分離し、非本番 → 本番の順で同じコードを昇格させる

## 認証(一時権限)

- CI からのクラウド認証は **OIDC フェデレーション**で行い、静的なアクセスキーを secrets に置かない
- 実行に使うロールは環境ごとに分け、plan 用(読み取り中心)と apply 用(書き込み)で権限を分離する

## バージョン固定(supply-chain-security と接続)

- Terraform 本体・provider・外部 module のバージョンを固定する(`required_version`・`required_providers` のピン、lock ファイルのコミット)
- 外部 module の参照はバージョン固定し、社外 module は導入前にレビューする(依存導入のクールダウンと同じ扱い)

## ドリフト検知

- 定期実行(スケジュールされた `terraform plan`)で宣言と実態の乖離を検知し、差分があれば issue 化して放置しない

## 構成の分割

- 変更頻度・影響範囲の異なるリソース群(ネットワーク基盤 / データストア / アプリケーション)は state を分割し、blast radius を限定する
