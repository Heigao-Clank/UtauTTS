# UtauTTS

UTAUボイスバンクの原音の選択と自然なつなぎ方を学習する日本語TTSの実験プロジェクトです。

標準レンダラは`waveform`です。原音の発音と声質を保つことを優先し、ピッチ加工を行わず接続します。比較研究用として次のレンダラも利用できます。

- `waveform`: 依存のない決定的な波形接続。推奨・明瞭度基準。
- `waveform-long`: 同一録音の連続原音をまとめる実験方式。AB/ABX評価用。
- `worldline` / `worldline-hybrid*`: WORLDによるピッチ・スペクトル処理を調べる実験方式。

CV・VCV、複数の`oto.ini`、UTF-8・Shift_JIS、`prefix.map`に対応しています。CVVCのVC挿入は未対応です。

## 現行仕様

- [構成](architecture.md): 合成パイプライン、原音選択、レンダラの位置付け
- [音源](voicebank.md): 対応ボイスバンクと同梱音源のライセンス
- [HTTPサーバー](server.md): 起動方法とAPI

## 学習と評価

- [接続学習データセット](connection-dataset.md): join cost用の境界ペア生成
- [モーラ長・韻律の学習](training.md): duration、F0、energyモデル
- [系列イントネーションモデル](intonation-model.md): TCNとアクセント特徴の研究経緯
- [接続品質の評価](evaluation.md): クリック、RMS、スペクトル、F0差
- [AB・ABX聴感評価](listening-test.md): ブラインド比較と集計

## 実験結果

- [接続モデル聴感評価](experiments/2026-08-10-listening/README.md)
- [レンダラ・イントネーション評価](experiments/2026-08-10-rendering/README.md)

## リリースビルド

Windows x64、Go、.NET 8 SDK、インターネット接続が必要。

```powershell
.\build.bat
```

次の成果物を生成します。

- `release/UtauTTS-win-x64.zip`: GUI・CLI・診断・学習ツール
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

## CLI

GUI版に同梱された足立レイを使う実行例です。`--renderer`を省略しても`waveform`になります。

```powershell
.\UtauTTS\utautts-cli.exe `
  --renderer waveform `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --out "out.wav"
```

イントネーションや学習ピッチをレンダラへ直接反映する経路は研究用です。リサンプルと時間伸縮によるケロケロ感や、WORLDによる子音脱落が確認されているため、明瞭度を評価するときは`waveform`と`--intonation-strength 0`を基準にしてください。

```powershell
.\UtauTTS\utautts-cli.exe `
  --renderer waveform `
  --intonation-strength 0.6 `
  --apply-pitch `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --out "out.wav" `
  --plan-out "plan.json"
```

配布物内のWORLDライブラリとブリッジは自動検出します。

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
go run ./cmd/connection-benchmark --voicebank "release/UtauTTS/voice/足立レイver3.5.0" --join-model "out/join-model.json" --corpus "docs/evaluation-corpus.json" --out "out/benchmark.json"
```
