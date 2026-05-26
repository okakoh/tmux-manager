# tmux-manager

> **Other Languages**: [English](README.md) | **日本語**

[![Go Version](https://img.shields.io/github/go-mod/go-version/okakoh/tmux-manager)](https://golang.org)
[![Release](https://img.shields.io/github/v/release/okakoh/tmux-manager)](https://github.com/okakoh/tmux-manager/releases)
[![License](https://img.shields.io/github/license/okakoh/tmux-manager)](LICENSE)

`tmux-manager` は、プロジェクトごとの tmux 作業環境を起動、再接続、再構築するための Go 製ターミナルUI です。

## 主な機能

- 🚀 **設定不要で開始**: 空の設定でも動作、使いながら設定を構築
- 🔄 **セッション方針**: 既存セッションに接続、確認、または再作成
- 📁 **プロジェクト中心**: 個別のtmuxコマンドではなく、プロジェクト単位で整理
- ⚡ **単体バイナリ**: デーモンなし、tmux以外の依存関係なし
- 🛠️ **再利用可能なツール**: プロジェクト間でツール定義を共有

このリポジトリは、作者が個人利用している道具を公開している段階のものです。まだ開発途中なので、挙動や設定形式が小さく変わる可能性があります。実際の開発作業で使いながら整えているツールとして見てもらえると助かります。

英語での Issue や相談も歓迎です。

## 目次

- [クイックスタート](#クイックスタート)
- [ユースケース](#ユースケース)
- [インストール](#インストール)
- [設定ファイル](#設定ファイル)
- [TUI の操作](#tui-の操作)
- [コントリビュート](#コントリビュート)

## クイックスタート

```sh
# インストール
go install github.com/okakoh/tmux-manager/cmd/tmux-manager@latest

# 空の設定で実行して開始
tmux-manager
# 's' を押して設定画面を開き、最初のプロジェクトを追加
```

## 設計方針

- **プロジェクト単位で扱う**: tmux コマンドの断片ではなく、作業対象のプロジェクトを中心にセッションを管理します。
- **設定を見える場所に置く**: 使い回すツールやプロジェクト構成は YAML に保存します。
- **安全側に倒す**: セッションの再作成や削除には確認を挟めます。
- **中身は普通の tmux**: `tmux` CLI を呼び出します。`.tmux.conf` は書き換えません。
- **単体バイナリ**: デーモンや常駐サービスはありません。

## ユースケース

複数のリポジトリを行き来して開発していると、毎回同じような tmux ウィンドウを開くことになります。エディタ、シェル、テスト、ログ、AI コーディングエージェントなどです。`tmux-manager` は、その構成を YAML に書いておき、ターミナルUI からプロジェクト単位で起動・接続できるようにします。

**Web 開発**
- 開発サーバー、エディタ、テスト、ファイルマネージャー

**Go プロジェクト**
- メインエディタ、テストランナー、air/ホットリロード、データベースコンソール

**データサイエンス**
- Jupyter、エディタ、データビューアー、モデル訓練ログ

**AI 開発**
- Codex、Claude Code、Hermes Agent などの AI コーディングエージェント
- Git や package scripts を実行するための通常のシェル
- スマホやリモート端末からのファイル確認用ツール

これを毎回手で開く代わりに、`tmux-manager` では「このプロジェクトではこのウィンドウを開く」という構成を保存できます。ターミナルUI でプロジェクトを選ぶだけで、同じ名前の tmux ウィンドウが揃います。すでにセッションがある場合は、そのまま接続する、確認してから再作成する、常に再作成する、といった方針もプロジェクトごとに指定できます。

たとえば Next.js アプリなら、次のような設定が考えられます。

```yaml
tools:
  server:
    window: server
    command: pnpm dev
    after_exit: shell
  codex:
    window: codex
    command: codex
    after_exit: shell
  claude:
    window: claude
    command: claude
    after_exit: shell
  hermes:
    window: hermes
    command: hermes
    after_exit: shell
  files:
    window: files
    command: yazi
    after_exit: shell

projects:
  - name: sample-next-app
    path: ~/src/sample-next-app
    session: sample-next-app
    default_window: server
    window_selection: prompt
    on_existing: prompt
    confirm_kill: true
    failure_policy: continue
    tools:
      - server
      - codex
      - claude:
          enabled: false
      - hermes:
          enabled: false
      - files
```

`server` や `codex` のようなツール定義は使い回せます。プロジェクト側では、セッション名、最初に開くウィンドウ、有効にするツール、既存セッションへの対応方針だけを変えられます。

スマホやリモート端末からファイルを確認するときに `yazi` を同じセッション内に置いておく、AI エージェントを複数候補として登録して必要なものだけ有効にする、といった使い方もしやすくなります。

## 用語

- **Project**: 作業対象のプロジェクト。パス、tmux セッション名、デフォルトウィンドウ、使用するツールを持ちます。
- **Tool**: `editor`、`shell`、`tests`、`logs` のような再利用できるウィンドウ定義です。
- **Window**: ツールから作られる tmux ウィンドウです。
- **Session**: プロジェクトに対応する tmux セッションです。
- **Default window**: セッションに接続する前に選択されるウィンドウです。
- **Existing session policy**: 対象の tmux セッションがすでにあるときに、接続するか、確認するか、作り直すかの方針です。

## 必要なもの

- ソースからビルドする場合は Go 1.26.3 以上の互換ツールチェーン
- `PATH` 上の `tmux`
- `$SHELL` または `PATH` 上の `sh` で解決できる POSIX 系シェル

ツールコマンドは、解決されたシェルの `-lc` 経由で実行されます。
`tmux-manager` は特定の `tmux` バージョンをインストール、同梱、要求しません。
設定ファイルで `tmux_binary` を指定しない限り、`PATH` 上で最初に見つかる
`tmux` を使います。

## インストール

### Go Install

公開済みの最新版を入れる場合:

```sh
go install github.com/okakoh/tmux-manager/cmd/tmux-manager@latest
```

Go のインストール先が `PATH` に入っている必要があります。通常は次の場所です。

```sh
$(go env GOPATH)/bin
```

### ソースからビルド

```sh
git clone https://github.com/okakoh/tmux-manager.git
cd tmux-manager
go build -o ./tmux-manager ./cmd/tmux-manager
./tmux-manager
```

バージョン確認:

```sh
tmux-manager -version
```

### Homebrew

```sh
brew install okakoh/tap/tmux-manager
```

Homebrew formula は、意図的に Homebrew の `tmux` formula へ依存しません。
多くのtmuxユーザーは既にtmux serverを起動しています。`tmux-manager`のインストール時にtmux clientのみが自動更新されると、既存のserverとバージョンが合わなくなる可能性があります。tmux を別途インストールまたは
更新した場合は、tmux server を再起動するか、その server と合う tmux client
を `tmux_binary` で指定してください。

更新:

```sh
brew upgrade tmux-manager
```

## 設定ファイル

デフォルトでは次のパスを読みます。

```text
$XDG_CONFIG_HOME/tmux-manager/config.yaml
```

`XDG_CONFIG_HOME` が未設定の場合は次のパスです。

```text
~/.config/tmux-manager/config.yaml
```

設定ファイルがない場合、TUI は空のプロジェクト一覧で起動します。`s` で設定画面を開き、プロジェクトやツールを追加して `Ctrl+S` で保存できます。

`PATH` 上で最初に見つかる `tmux` ではなく、特定の tmux バイナリを使う場合:

```yaml
tmux_binary: /opt/homebrew/Cellar/tmux/3.5a/bin/tmux
```

これは、起動中の tmux server を意図的に特定バージョンで維持している場合に
使えます。`tmux-manager` がその server を自動で終了または再起動することは
ありません。

設定ファイルは信頼済み入力として扱ってください。ツールの `command` は
実行可能なシェルコマンドなので、信頼できない場所からコピーした設定は
内容を確認してから使ってください。

サンプル設定から始める場合:

```sh
mkdir -p ~/.config/tmux-manager
cp examples/config.yaml ~/.config/tmux-manager/config.yaml
```

その後、サンプル内のプロジェクトパスを自分の環境に合わせて編集してください。

設定ファイルのパスを明示することもできます。

```sh
tmux-manager -config examples/config.yaml
```

## 設定例

### 最小構成（2ウィンドウ）
```yaml
tools:
  edit:
    window: edit
    command: nvim
    after_exit: shell
  term:
    window: term
    command: bash
    after_exit: shell

projects:
  - name: my-project
    path: ~/my-project
    session: my-project
    default_window: edit
    window_selection: configured
    on_existing: attach
    confirm_kill: true
    failure_policy: stop
    tools:
      - edit
      - term
```

### 開発サーバーを追加
```yaml
# 前のツール定義 +
tools:
  serve:
    window: serve
    command: npm run dev
    after_exit: shell

# プロジェクトのツールに 'serve' を追加
projects:
  - name: my-web-app
    # ... その他の設定
    tools:
      - edit
      - term
      - serve
```

ツールコマンドは、解決されたシェルで次の形式で実行されます。

```sh
sh -lc "<command>; exec sh"
```

そのため、コマンド終了後もシェルが残ります。

## TUI の操作

ホーム画面:

- `Enter`: 停止中のプロジェクトを起動、または起動済みプロジェクトに接続
- `r`: 選択中のセッションを再作成
- `k`: 選択中のセッションを終了
- `w`: 起動・接続前に対象ウィンドウを選択
- `/`: プロジェクトを絞り込み
- `s`: 設定画面を開く
- `b`: tmux キーバインド一覧を表示
- `?`: ヘルプ
- `q`: 終了

設定画面:

- `Tab`: プロジェクト編集とグローバルツール編集を切り替え
- `Up/Down` または `j/k`: フィールド移動
- `Left/Right` または `h/l`: 選択中のプロジェクト・ツールを切り替え
- `Enter`: 選択フィールドを編集、または値を切り替え
- プロジェクトのツール行で `Enter` または `Space`: ツールの追加、有効・無効の切り替え
- プロジェクトのツール行で `d`: そのプロジェクトからツール参照を削除
- `a`: プロジェクトまたはツールを追加
- アクション行で `d`: プロジェクトまたはツールを削除
- `Ctrl+S`: 検証して保存
- `x`、`Esc`、`q`: 変更を破棄

キーバインド画面:

- `b`: キーバインド一覧を再読み込み
- `q` または `Esc`: ホーム画面へ戻る

キーバインド画面は `tmux list-keys` を呼び出すだけで、tmux の設定は変更しません。

## ポリシー

`window_selection`:

- `configured`: `default_window` を使う
- `prompt`: 実行前に TUI でウィンドウを選ぶ

`on_existing`:

- `attach`: 既存セッションに接続
- `prompt`: 接続するか再作成するか確認
- `recreate`: セッションを終了して作り直す

`failure_policy`:

- `stop`: tmux 操作が失敗した時点で停止
- `continue`: 最終ウィンドウ以外の作成失敗は続行し、部分成功として扱う

## 開発

```sh
go test ./...
go vet ./...
go build ./cmd/tmux-manager
```

tmux を使った手動確認をする場合は、重要な既存セッションとは別の名前を使い、確認後に削除してください。

## コントリビュート

Issue、バグ報告、機能要望、設定例、ドキュメント修正を歓迎します。報告時は、可能な範囲で次の情報を含めてください。

- OS と tmux のバージョン
- `tmux-manager` のバージョンまたは commit
- 設定や起動挙動に関する報告の場合、最小限の設定例
- 期待した挙動と実際の挙動

日本語での報告も歓迎です。必要であれば、英語のタイトルだけ短く付けて本文は日本語で書いてください。

## プライバシー

個人のプロジェクトパス、非公開のコマンド引数、API キー、ローカル設定のバックアップをコミットしないでください。実際の設定は `~/.config/tmux-manager/config.yaml` に置き、`examples/` は汎用的な内容にしてください。

## ライセンス

MIT
