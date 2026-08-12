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

## GUIの拡張設定

Windows GUIは、実行ファイルと同じディレクトリ、または起動時の作業ディレクトリにある`gui-config.json`を読み込みます。`gui-config.example.json`をコピーして、`renderers`にプルダウンへ表示するRenderer、`models`に抑揚モデルを追加できます。`label`が表示名、`backend`が内部Renderer名、`path`がモデルJSONのパスです。相対パスは設定ファイルの場所を基準に解決されます。

`executable_kind`は既存の外部実行ファイルを指定するための項目で、`worldline`、`bridge`、`resampler`に対応します。新しい任意形式の実行ファイルを追加する場合は、同じbackend識別子をRenderer側にも実装する必要があります。

## 実験結果

- [接続モデル聴感評価](docs/experiments/2026-08-10-listening/README.md)
- [レンダラ・イントネーション評価](docs/experiments/2026-08-10-rendering/README.md)

## リリースビルド

Windows x64、Go、.NET 8 SDK、Python 3.12 x64、インターネット接続が必要。PythonとPyInstallerはOpenJTalk helperのビルド時だけ使用し、完成した配布物の実行には不要です。

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

`--voice-dir`で音源格納ディレクトリ、`--voicebank`で初期選択、`--text`、`--out`で入力欄の初期値を指定できます。

GUIの「生成」は音声をプレビュー用に保持し、「再生」でウィンドウ内から再生できます。「名前を付けて保存」から保存先を選び、WAVを書き出します。生成前に保存先を入力する必要はありません。

メイン画面の「抑揚モデル」プルダウンには、配布物の`models/`にある有効なモデルJSONを自動列挙します。「なし」は原音のピッチを維持します。frame pitchモデルを選ぶと、対応する`OpenUTAU Classic faithful`へ音声モードを自動で切り替えます。今後モデルを追加するときも、対応形式のJSONを`models/`へ配置することで選択肢に追加できます。

「詳細設定」では、読み、音高、モーラ長、休止、音素選択、結合モデル、Worldline関連実行ファイル、境界補修を変更できます。OpenJTalk特徴は自動生成されるため、特徴JSONや特徴ケースをGUIで選ぶ必要はありません。

ローカルの`out/prosody`にv8モデルとアクセント特徴がある場合、Windows配布物の`models/`へ自動同梱します。成果物がない環境では、詳細設定からJSONを選択してください。

## CLI

GUI版に同梱された足立レイを使う実行例です。`--renderer`を省略しても`waveform`になります。

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
  --prosody ".\UtauTTS\models\frame-intonation-v8.json" `
  --prosody-pitch-only `
  --out "v8.wav"
```

v9/v9.1句アンカー補正モデルも同じ`openutau-classic-worldline-faithful` rendererで使用できます。モデルJSONを`models/`へ置くとGUIの「抑揚モデル」に自動表示されます。v9.1はOpen JTalk整列と平滑化log-F0教師を強化した学習版です。

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
