# supply-chain-pnpm: pnpm での実現手法

[supply-chain-security](../adr/supply-chain-security.md) プリセットの原則を pnpm プロジェクトで実現する手法集(非規範)。

## 導入のクールダウン

- `pnpm-workspace.yaml`(または `.npmrc`)に `minimumReleaseAge` を設定する(目安 7 日 = 10080 分)。公開直後のバージョンがインストール解決の対象にならなくなる
- 例外(セキュリティ修正の即時採用)は `minimumReleaseAgeExclude` に人間承認の上で追加し、対応後に削除する

## 依存宣言の一元管理

- ワークスペースの **catalog**(`pnpm-workspace.yaml` の `catalog:`)にバージョンを集約し、各 package.json は `catalog:` 参照にする。バージョンの宣言箇所を 1 箇所にする
- 直接依存は exact 指定(`save-exact=true`)にし、範囲指定を野放しにしない

## CI の凍結インストール

- CI では `pnpm install --frozen-lockfile` を使う(lockfile と宣言の不一致で fail)。`pnpm-lock.yaml` は必ずコミットする

## ツールチェーンの固定

- `packageManager` フィールド(corepack)で pnpm 自体のバージョンを固定する

## 脆弱性への対応

- dependabot(npm ecosystem)+ cooldown 設定で更新 PR を自動化する。既知脆弱性は `pnpm audit` を CI で実行して検出し、通常の PR フローで対処する
- 推移的依存の脆弱バージョンは `overrides` で暫定固定できる(恒久対応は更新 PR)
