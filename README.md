# UtauTTS

UTAUボイスバンクを再学習せず、日本語TTSとして利用する実験プロジェクトです。

レンダラは次の三つを利用できます。

- `waveform`: 依存のない決定的な波形接続。
- `waveform-long`: 同一録音の連続原音をまとめる実験方式。AB/ABX評価用。
- `worldline-hybrid`: WORLDで母音を合成し、弱くなった子音だけ原波形から復元。推奨

CV・VCV、複数の`oto.ini`、UTF-8・Shift_JIS、`prefix.map`に対応しています。CVVCのVC挿入は未対応です。

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

## CLI

GUI版に同梱された足立レイを使う、推奨レンダラの実行例です。

```powershell
.\UtauTTS\utautts-cli.exe `
  --renderer worldline-hybrid `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --out "out.wav"
```

配布物内のWORLDライブラリとブリッジは自動検出します。イントネーション補正は`0`から`1`で指定できます。

```powershell
.\UtauTTS\utautts-cli.exe `
  --renderer worldline-hybrid `
  --intonation-strength 0.6 `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --out "out.wav" `
  --plan-out "plan.json"
```

WORLDを使わない場合は`--renderer waveform`を指定するか、オプションを省略します。

## HTTP API

```powershell
.\UtauTTS-Server\utautts-server.exe `
  --voice-dir ".\UtauTTS\voice" `
  --renderer worldline-hybrid
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
```

同梱音源とライセンスは[docs/voicebank.md](docs/voicebank.md)、構成は[docs/architecture.md](docs/architecture.md)、モーラ長の学習は[docs/training.md](docs/training.md)、接続の測定は[docs/evaluation.md](docs/evaluation.md)、接続データ生成は[docs/connection-dataset.md](docs/connection-dataset.md)、AB/ABX評価は[docs/listening-test.md](docs/listening-test.md)を参照してください。
