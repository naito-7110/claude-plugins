---
description: Analyze project structure and generate README.md
tools:
  - Bash(ls, find, cat, head)
  - Glob
  - Read
  - Write
  - AskUserQuestion
---

プロジェクト構造を分析し、README.mdを生成します。

## 手順

1. プロジェクト構造を分析
   - `package.json`, `Cargo.toml`, `pyproject.toml` 等の設定ファイルを確認
   - ディレクトリ構造を把握
2. 主要ファイルを読み取り、プロジェクトの目的を理解
3. 既存のREADMEがあれば確認
4. README.mdを生成（または更新）

## 生成するセクション

- プロジェクト名と概要
- インストール方法
- 使用方法
- 主要機能
- ディレクトリ構造（必要に応じて）
- ライセンス

## 注意事項

- 既存のREADMEがある場合は上書き確認を行う
- プロジェクトの言語/フレームワークに応じた適切な形式で生成
