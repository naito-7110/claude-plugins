# supply-chain-pnpm: pnpm での実現手法

[supply-chain-security](../adr/supply-chain-security.md) プリセットの原則を pnpm プロジェクトで実現する手法集(非規範)。

## 防御の全体像(3 層)

| 層 | 方針 | 手段 | 防御対象 |
| --- | --- | --- | --- |
| 1 | 入れない | minimumReleaseAge・trustPolicy | 公開直後の悪意あるバージョン(ゼロデイ) |
| 2 | 実行させない | ignore-scripts + ビルド許可リスト | インストールスクリプト経由の侵害 |
| 3 | 見逃さない | audit・OSV スキャンを CI で | 既知の脆弱性 |

実際のサプライチェーン侵害では、悪意あるバージョンは公開から数時間〜数日で検知・修正されることが多く、**遅延(1 層)とスクリプト無効化(2 層)だけで実被害の大半を回避できる**。3 層(SCA)は既知の脆弱性しか検知できない点を理解して併用する。

## 1 層: 入れない

`pnpm-workspace.yaml`:

```yaml
minimumReleaseAge: 10080   # 公開から 7 日経過するまでインストールを拒否
trustPolicy: no-downgrade  # バージョンのダウングレードを拒否
```

- 例外(セキュリティ修正の即時採用)は `minimumReleaseAgeExclude` に人間承認の上で追加し、対応後に削除する
- さらに強くする場合は、脅威インテリジェンス型のローカルプロキシ(既知の悪意あるパッケージのブロック・新規公開の自動保留)の導入を検討する

## 2 層: 実行させない

`.npmrc`:

```ini
ignore-scripts=true
```

- サプライチェーン侵害の実被害の多く(環境変数・SSH 鍵・npm トークンの窃取)は **preinstall / postinstall スクリプト**で起きる。スクリプト実行を既定で止める
- ネイティブモジュール等でビルドスクリプトが必要なものだけ、pnpm の `onlyBuiltDependencies` の**許可リスト**で明示する:

```json
{ "pnpm": { "onlyBuiltDependencies": ["esbuild", "sharp"] } }
```

## 依存宣言の一元管理

- ワークスペースの **catalog**(`pnpm-workspace.yaml` の `catalog:`)にバージョンを集約し、各 package.json は `catalog:` 参照にする。バージョンの宣言箇所を 1 箇所にする
- 直接依存は exact 指定(`save-exact=true`)にし、範囲指定を野放しにしない

## CI の凍結インストール

- CI では `pnpm install --frozen-lockfile` を使う(lockfile と宣言の不一致で fail)。`pnpm-lock.yaml` は必ずコミットする

## ツールチェーンの固定

- `packageManager` フィールド(corepack)で pnpm 自体のバージョンを固定する
- nix を使うプロジェクトは devShell で node(LTS)/ pnpm を供給し、環境差を消す

## 導入前チェック(AI 時代の追加観点)

- **実在確認**: AI コーディングツールは実在しないパッケージ名を提案しうる(その名前を攻撃者が先回りで公開する slopsquatting の標的になる)。導入前にレジストリで実在と正確な綴りを確認する
- 単一メンテナーのパッケージはアカウント乗っ取りの単一障害点になる。コントリビューター数・ダウンロード実績・最終更新日を確認する
- 選定基準の一般原則(ライセンス含む)は dependency-licensing プリセット側で定める

## 3 層: 見逃さない(CI)

- `pnpm audit` に加え、複数ソースを統合した脆弱性 DB を使うスキャナ(OSV スキャナ等)を CI に組み込み、脆弱な依存のマージを止める
- 推移的依存の脆弱バージョンは `overrides` で暫定固定できる(恒久対応は更新 PR)
- 更新の自動化は dependabot(npm ecosystem)+ cooldown

## 参考

- 『バイブコーディングの脆弱性』(Kyohei Fukuda 著)2.6 章 — 3 層防御の整理と実事例(2025 年の npm 大規模侵害・自己増殖型ワーム・AI ツール起点の攻撃)
