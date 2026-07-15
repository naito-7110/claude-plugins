# tools/atelier

atelier プラグインの CLI。GitHub Projects の正準ボードの複製・検証(`board copy` / `board verify`)、issue / PR の整合検証(`issue verify` / `pr verify`)、文書構造の検証(`docs verify`)を提供する。認証は gh CLI のセッションを継承する(go-gh)。

## ビルドとテスト

```console
$ cd tools/atelier
$ go build ./...
$ go test ./...
```

nix devShell(リポジトリルートの `flake.nix`)が Go ツールチェーン・golangci-lint・goreleaser を供給する。

## リリース(配布)

リリースは **`atelier/vX.Y.Z` 形式のタグ push** で行う(人間の操作。CI からは発火しない。**先に plugin.json + CHANGELOG の版上げ PR を main に入れる**)。AI とオーナーは同一の gh 認証を共有しサーバー側で区別できないため、リリースの便利コマンドは提供しない(#129 — 引き金は人間が手で打つ生 git のみ。AI からのタグ push は hook が塞ぐ):

```console
$ git fetch --tags origin
$ git tag -l 'atelier/v*'                       # 既存タグと重複しないことを確認
$ git tag atelier/v1.0.0 origin/main            # 対象 SHA は常にリモートの ref から
$ git push origin atelier/v1.0.0
```

タグ push で `.github/workflows/atelier-release.yml` が goreleaser を実行し、GOOS={linux, darwin, windows} × GOARCH={amd64, arm64} の 6 バイナリ(アーカイブ)と `checksums.txt` を GitHub Release に添付する。ビルド設定は `tools/atelier/.goreleaser.yml`。

続けて同 workflow の `pin-update` ジョブが、公開された Release の `checksums.txt` を取得してプラグイン同梱ピン(`plugins/atelier/pin/version` + `pin/checksums.txt`)を新リリースへ追従させる**更新 PR を自動で起こす**。ピンは「レビュー済み commit = 信頼の根」(ADR 0003)なので main へ直接は書かない。**この PR を人間がマージすると、各マシンの SessionStart ブートストラップが新ピンへ収束する**(ブートストラップの詳細は `plugins/atelier/hooks/README.md`)。手動でのピン更新は不要。

> ジョブが PR を作成するには、リポジトリ設定の **Settings → Actions → General → Workflow permissions → 「Allow GitHub Actions to create and approve pull requests」** が有効である必要がある。

## 取得

`gh release download` で OS / arch に合ったアーカイブと checksum を取得し、検証してから展開する:

```console
$ gh release download atelier/v1.0.0 --repo naito-7110/claude-plugins \
    --pattern 'atelier_*_linux_amd64.tar.gz' --pattern 'checksums.txt'
$ shasum -a 256 --check --ignore-missing checksums.txt
atelier_1.0.0_linux_amd64.tar.gz: OK
$ tar -xzf atelier_1.0.0_linux_amd64.tar.gz atelier
```

アーカイブ名は `atelier_<version>_<os>_<arch>.tar.gz`(windows は `.zip`)。os は `linux` / `darwin` / `windows`、arch は `amd64` / `arm64`。

