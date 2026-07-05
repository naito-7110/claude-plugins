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

リリースは **`atelier/vX.Y.Z` 形式のタグ push** で行う(人間の操作。CI からは発火しない):

```console
$ git tag atelier/v0.3.0
$ git push origin atelier/v0.3.0
```

タグ push で `.github/workflows/atelier-release.yml` が goreleaser を実行し、GOOS={linux, darwin, windows} × GOARCH={amd64, arm64} の 6 バイナリ(アーカイブ)と `checksums.txt` を GitHub Release に添付する。ビルド設定は `tools/atelier/.goreleaser.yml`。

## 取得

`gh release download` で OS / arch に合ったアーカイブと checksum を取得し、検証してから展開する:

```console
$ gh release download atelier/v0.3.0 --repo naito-7110/claude-plugins \
    --pattern 'atelier_*_linux_amd64.tar.gz' --pattern 'checksums.txt'
$ shasum -a 256 --check --ignore-missing checksums.txt
atelier_0.1.0_linux_amd64.tar.gz: OK
$ tar -xzf atelier_0.1.0_linux_amd64.tar.gz atelier
```

アーカイブ名は `atelier_<version>_<os>_<arch>.tar.gz`(windows は `.zip`)。os は `linux` / `darwin` / `windows`、arch は `amd64` / `arm64`。

