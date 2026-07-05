# tools/factory

factory プラグインの CLI。GitHub Projects の正準ボードの複製・検証(`board copy` / `board verify`)、issue / PR の整合検証(`issue verify` / `pr verify`)、文書構造の検証(`docs verify`)を提供する。認証は gh CLI のセッションを継承する(go-gh)。

## ビルドとテスト

```console
$ cd tools/factory
$ go build ./...
$ go test ./...
```

nix devShell(リポジトリルートの `flake.nix`)が Go ツールチェーン・golangci-lint・goreleaser を供給する。

## リリース(配布)

リリースは **`factory/vX.Y.Z` 形式のタグ push** で行う(人間の操作。CI からは発火しない):

```console
$ git tag factory/v0.1.0
$ git push origin factory/v0.1.0
```

タグ push で `.github/workflows/factory-release.yml` が goreleaser を実行し、GOOS={linux, darwin, windows} × GOARCH={amd64, arm64} の 6 バイナリ(アーカイブ)と `checksums.txt` を GitHub Release に添付する。ビルド設定は `tools/factory/.goreleaser.yml`。

## 取得

`gh release download` で OS / arch に合ったアーカイブと checksum を取得し、検証してから展開する:

```console
$ gh release download factory/v0.1.0 --repo naito-7110/claude-plugins \
    --pattern 'factory_*_linux_amd64.tar.gz' --pattern 'checksums.txt'
$ shasum -a 256 --check --ignore-missing checksums.txt
factory_0.1.0_linux_amd64.tar.gz: OK
$ tar -xzf factory_0.1.0_linux_amd64.tar.gz factory
```

アーカイブ名は `factory_<version>_<os>_<arch>.tar.gz`(windows は `.zip`)。os は `linux` / `darwin` / `windows`、arch は `amd64` / `arm64`。

## tick の準リアルタイム運転(推奨構成)

`factory tick run` は claude を起動する前に、mode gate → 多重起動ロック → **作業検知プリチェック**(Ready issue / 未対応レビュースレッド / factory-review failure / 未回収 merged PR — すべて Go + gh API、1 tick あたり 3 クエリ)を通す。**作業が無ければ claude セッションを一切立てない**(#111)ため、tick は短周期にしてよい:

```console
# 準リアルタイム(1 分 cron): イベントへの反応が体感 1〜2 分になる
* * * * * cd /path/to/repo && /path/to/factory tick run >> .agents/night.log 2>&1
```

```bash
# 同等の常駐ラッパー(cron が使えない環境)
while true; do
  factory tick run >> .agents/night.log 2>&1 || true
  sleep 60
done
```

- 空振り時のコストは gh API 3 クエリのみ(1 分周期でも 180 req/h — GraphQL レート制限に対して十分小さい)。claude 起動ゼロ・ログ 1 行
- 前回起動時刻は `.agents/tick-state`(ローカル・非コミット)に記録し、**claude を起動したときだけ**更新する(空振りで進めない — 取りこぼし防止)
- `factory tick install` の既定スケジュール(平日 3:00)は夜間バッチの例。準リアルタイムにするなら `--schedule "* * * * *"` を渡す
