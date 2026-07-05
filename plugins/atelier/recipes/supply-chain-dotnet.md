# supply-chain-dotnet: .NET での実現手法

[supply-chain-security](../adr/supply-chain-security.md) プリセットの原則を .NET プロジェクトで実現する手法集(非規範)。

## 依存宣言の一元管理

- **中央パッケージ管理**を有効にする: `Directory.Packages.props` に `ManagePackageVersionsCentrally=true` + 全 `PackageVersion` を集約し、各 csproj は `PackageReference`(バージョン記述なし)だけにする。バージョンの宣言箇所を 1 箇所にする
- floating version(`*` 指定)を使わない

## CI の凍結インストール

- lock ファイルを有効化する: `RestorePackagesWithLockFile=true` で `packages.lock.json` を生成・コミットし、CI の restore は `--locked-mode` で実行する(lock と宣言の不一致で fail)

## 導入のクールダウン

- NuGet 側に公開経過日数の設定が無いため、**dependabot(nuget ecosystem)の cooldown 設定**でクールダウンを代替する。手動追加時は公開日をレビュー観点にする(supply-chain-security の原則どおり公開直後の採用は人間承認)

## フィードの固定

- `nuget.config` をリポジトリにコミットし、パッケージソースを明示・固定する。**Package Source Mapping** でパッケージ名とソースの対応を固定し、依存混同(dependency confusion)を防ぐ

## 脆弱性への対応

- `dotnet list package --vulnerable --include-transitive` を CI で実行して検出し、通常の PR フローで対処する

## ツールチェーンの固定

- `global.json` で SDK バージョンをピンし、リポジトリルートに置く(範囲外 SDK で fail-fast)
