# レシピ集

プリセット ADR(規範)の原則を、特定のスタック・トランスポート・プラットフォームで**どう実現するか**の非規範マッピング集。

## 位置づけ

- **プリセット** = 何を守るか(規範。改訂は改憲プロセス)
- **レシピ** = このスタックではどう実現するか(知識。改訂は通常の PR で軽量に)
- レシピはプリセット同様プラグイン同梱の参照モデルで、plugin 更新により全プロジェクトへ配布される

## 使われ方

- `/factory:init` がスタック事実の導出時に、検出したスタックに該当するレシピを参照して適用を**提案**する(強制はしない)
- work / groom も実装・仕様検討時の参考として読む
- 新しい手法・対策を見つけたら、レシピへの追記 PR で中央に蓄積する

## 命名

`<topic>-<stack>.md`。topic は対応するプリセットの slug に揃える(例: `supply-chain-pnpm.md`、`error-handling-http.md`)。

## 収録一覧

| レシピ | 対応プリセット | 状態 |
| --- | --- | --- |
| [error-handling-http](./error-handling-http.md) | error-handling | 収録済み |
| [supply-chain-pnpm](./supply-chain-pnpm.md) | supply-chain-security | 収録済み |
| [supply-chain-dotnet](./supply-chain-dotnet.md) | supply-chain-security | 収録済み |
| [supply-chain-gha](./supply-chain-gha.md) | supply-chain-security | 収録済み |
| [i18n-typescript](./i18n-typescript.md) | i18n-copy | 収録済み |
| [infrastructure-terraform](./infrastructure-terraform.md) | infrastructure | 収録済み |
