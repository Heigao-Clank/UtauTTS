# UtauTTS

UTAUボイスバンクの原音の選択と自然なつなぎ方を学習する日本語TTSの実験プロジェクトです。

---

UtauTTSは声そのものをニューラルネットワークで生成するTTSではありません。UTAUボイスバンクに収録された声質と発音を残し、文章に合う原音の選択・接続・ピッチ操作を別々の処理として組み合わせています。

```text
入力文章
  ↓
読みとモーラへ変換
  ↓
UTAUボイスバンクから原音を選択
  ↓
原音の長さと接続位置を決定
  ↓
選択した抑揚モデルでピッチ曲線を予測
  ↓
rendererが原音を接続し、必要ならピッチ曲線を適用
  ↓
音声ファイルへ
```

各部分の役割は次のとおりです。

| 部分 | 役割 |
|---|---|
| UTAUボイスバンク | 声質、発音、子音、母音を提供する |
| 日本語フロントエンド | 漢字を読みに変換し、OpenJTalkでアクセント特徴を作り、モーラへ分解する |
| 原音選択 | `oto.ini`、`prefix.map`から使用するWAVを決める |
| 接続計画 | preutterance、overlap、モーラ長、休止位置を決める |
| 言語特徴 | アクセント句、アクセント核、単語境界、品詞を表す |
| 抑揚モデル | 言語特徴と発話位置から、時間ごとの相対ピッチを予測する |
| renderer | 原音を実際に接続し、ピッチ曲線を音声へ反映する |

したがって、抑揚モデルを変更しても声質の出所はUTAUボイスバンクのままです。モデルはどんな声を出すかではなく、収録音声を時間方向にどう上下させるかを担当します。

### 現在採用している構成

イントネーションなしの基準はwaveform rendererです。原音へピッチ加工を行わないため、ボイスバンクの声質と発音を最もそのまま確認できます。

学習イントネーションを使う場合は、v8 frame TCNモデルと`openutau-classic-worldline-faithful` rendererを組み合わせます。v8が10ms単位の相対ピッチ曲線を作り、faithful rendererがOpenUTAUに近いphoneme timing、5点エンベロープ、Worldline resamplingでその曲線を適用します。v8は音声波形や話者の声を生成しません。

### v8と言語特徴

v8はモーラごとのアクセント句、アクセント核、単語境界、品詞を必要とします。モデル選択時は、同梱したOpenJTalk frontendが入力文章から読みとこれらの特徴を実行時に生成します。得られたモーラ列はGo側の解析結果と照合し、ずれた特徴を音声へ適用しません。

標準レンダラは`waveform`です。原音の発音と声質を保つことを優先し、ピッチ加工を行わず接続します。比較研究用として次のレンダラも利用できます。

- `waveform`: 依存のない決定的な波形接続。推奨・明瞭度基準。
- `waveform-long`: 同一録音の連続原音をまとめる実験方式。AB/ABX評価用。
- `worldline` / `worldline-hybrid*`: WORLDによるピッチ・スペクトル処理を調べる実験方式。

CV・VCV、複数の`oto.ini`、UTF-8・Shift_JIS、`prefix.map`に対応しています。CVVCのVC挿入は未対応です。

## 現行仕様

- [構成](docs/architecture.md): 合成パイプライン、原音選択、レンダラの位置付け
- [モデル／Rendererプラグイン](docs/plugins.md): 自己記述manifest、追加方法、配布方法
- [音源](docs/voicebank.md): 対応ボイスバンクと同梱音源のライセンス
- [HTTPサーバー](docs/server.md): 起動方法とAPI
- [境界補修実験](docs/boundary-repair.md): 母音末尾を使った接続補助の比較方法

## 学習と評価

- [接続学習データセット](docs/connection-dataset.md): join cost用の境界ペア生成
- [モーラ長・韻律の学習](docs/training.md): duration、F0、energyモデル
- [系列イントネーションモデル](docs/intonation-model.md): TCNとアクセント特徴の研究経緯
- [手動イントネーション](docs/manual-pitch.md): モーラごとの音高補正を編集するJSONとCLI/GUIの使い方
- [接続品質の評価](docs/evaluation.md): クリック、RMS、スペクトル、F0差
- [AB・ABX聴感評価](docs/listening-test.md): ブラインド比較と集計
- [今後の改善ロードマップ](docs/future-roadmap.md): 採用構成、任意文章対応、GUI統合の順序

## GUIの構成

標準GUIはQt 6 + Qt Quick/QML版です。Goバックエンドを共有ライブラリとして同一プロセスへ読み込み、C ABIを介して音源列挙・読み解析・合成を直接呼びます。GUI起動時のHTTPサーバーやWebViewは使用しません。配布物ではQt本体とネイティブバックエンドを`app/`へまとめ、ルートの`utautts.exe`から起動します。

## 実験結果

- [接続モデル聴感評価](docs/experiments/2026-08-10-listening/README.md)
- [レンダラ・イントネーション評価](docs/experiments/2026-08-10-rendering/README.md)

## リリースビルド

Windows x64、Go、Qt 6.5以降（Qt Quick、Qt Multimedia、Qt Concurrent）、CMake、Ninja、MSYS2 Clang、.NET 8 SDK、Python 3.12 x64、インターネット接続が必要です。Qt SDKを`.qt/<version>/mingw_64`へ配置した場合は自動検出します。それ以外の場所ではcompiler kitディレクトリを`QT_ROOT`へ設定してください。

```powershell
.\build.bat
```

次の成果物を生成します。

- `release/UtauTTS-win-x64.zip`: GUI本体、`tools/`内のCLI・診断・学習ツール、`runtime/`内の音声処理依存
- `release/UtauTTS-Server-win-x64.zip`: HTTPサーバーのみ

ビルド時にOpenUtau 0.1.565の`worldline.dll`を取得し、SHA-256を検証してから同梱します。ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。

## GUI

```powershell
.\UtauTTS\utautts.exe
```

実行ファイルと同じ場所にある`voice`ディレクトリにボイスバンクを解凍して配置してください。

GUI版にはデフォルト音源として足立レイ UTAU音源 ver3.5.0を同梱しています。

「再読込」を押すかUtauTTSを再起動することで、追加した音源を反映できます。

画面下部の再生ボタンは、未生成または編集済みならGoバックエンドをワーカースレッドで呼び、現在の音声が有効なら再生・一時停止を行います。「WAV保存」から現在の発話に対応するプレビュー音声を保存できます。標準のv8モデルと対応Rendererを使用するため、GUIのピッチ処理は既定で有効です。設定画面から無効にすると原音ピッチを維持します。

Qt GUIだけを開発ビルドする場合は次を使用します。

```powershell
$env:QT_ROOT = 'C:\Qt\6.8.3\mingw_64'
.\tools\build-qt.ps1
```

配布するモデルは`id`と`display_name`をモデルJSON自身に持たせ、`tools/install-prosody-model.ps1`で`models/`へ追加します。release buildは研究用の`out/prosody`から特定ファイル名を推測して同梱しません。

## CLI

GUI版に同梱された足立レイを使う実行例です。`--renderer`を省略すると、Renderer manifestで最も高い既定優先度を持つプラグインを使います。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --renderer waveform `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --out "out.wav"
```

イントネーションや学習ピッチをレンダラへ直接反映する経路は研究用です。リサンプルと時間伸縮によるケロケロ感や、WORLDによる子音脱落が確認されているため、明瞭度を評価するときは`waveform`と`--intonation-strength 0`を基準にしてください。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --renderer waveform `
  --intonation-strength 0.6 `
  --apply-pitch `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --out "out.wav" `
  --plan-out "plan.json"
```

配布物内の`runtime`にあるWORLDライブラリとブリッジは自動検出します。

v8を任意文章へ適用する例です。`--prosody-features`を省略すると、配布物内のOpenJTalk frontendが読みとアクセント特徴を自動生成します。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "駅前の新しい図書館で本を三冊借りました。" `
  --renderer openutau-classic-worldline-faithful `
  --prosody "<model-id>" `
  --prosody-pitch-only `
  --out "v8.wav"
```

自己記述モデルJSONを`models/`へinstallするとGUIの「抑揚モデル」に自動表示されます。モデル側の`recommended_renderers`で互換性のあるRenderer plugin IDを宣言できます。

## HTTP API

```powershell
.\UtauTTS-Server\utautts-server.exe `
  --voice-dir ".\UtauTTS\voice" `
  --renderer waveform
```

## 開発

```powershell
go test ./...
go run ./cmd/utautts-cli --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --text "こんにちは。" --out "out.wav"
go run ./cmd/oto-inspect --oto "release/UtauTTS/voice/足立レイver3.5.0"
go run ./cmd/connection-compare --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --text "こんにちは。" --out "out/compare"
go run ./cmd/connection-dataset --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --out "out/connections.jsonl"
go run ./cmd/connection-train --dataset "out/connections.jsonl" --out "out/join-model.json"
go run ./cmd/utautts-cli --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --text "こんにちは。" --join-model "out/join-model.json" --out "out/learned.wav"
go run ./cmd/connection-lattice --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --text "こんにちは。" --join-model "out/join-model.json" --out "out/lattice.json"
go run ./cmd/voicebank-capability --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --corpus "docs/evaluation-corpus.json" --out "out/capability.json"
go run ./cmd/connection-benchmark --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --join-model "out/join-model.json" --corpus "docs/evaluation-corpus.json" --out "out/benchmark.json"
go run ./cmd/boundary-benchmark --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --corpus "docs/evaluation-corpus.json" --out "out/boundary-benchmark.json"
```
