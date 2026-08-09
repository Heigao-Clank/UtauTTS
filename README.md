# UtauTTS

UTAUボイスバンクを再学習せず、日本語TTSとして利用する実験プロジェクトです。`oto.ini`から原音を選び、発話向けに長さ・接続・F0を調整します。

レンダラは次の二つを利用できます。

- `waveform`: 依存のない決定的な波形接続。
- `worldline-hybrid`: WORLDで母音を合成し、弱くなった子音だけ原波形から復元。現在の推奨

CV・VCV、複数の`oto.ini`、UTF-8・Shift_JIS、`prefix.map`に対応しています。CVVCのVC挿入は未対応です。

## リリースビルド

Windows x64、Go、.NET 8 SDK、インターネット接続が必要。

```powershell
.\build.bat
```

次の成果物を生成します。

- `release/UtauTTS-win-x64.zip`: GUI・CLI・診断・学習ツール
- `release/UtauTTS-Server-win-x64.zip`: HTTPサーバーのみ

ビルド時にOpenUtau 0.1.565の`worldline.dll`を取得し、SHA-256を検証して同梱します。ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。

## GUI

```powershell
.\UtauTTS\utautts.exe
```

ボイスバンクのフォルダと文章を指定して「生成」を押します。初期設定は`worldline-hybrid`、イントネーション`0.6`、出力先`output.wav`です。

`--voicebank`、`--text`、`--out`で入力欄の初期値を指定できます。

## CLI

ボイスバンクは配布物に含まれません。推奨レンダラの実行例です。

```powershell
.\UtauTTS\utautts-cli.exe `
  --renderer worldline-hybrid `
  --voicebank "sample\uta" `
  --text "こんにちは、今日はいい天気です。" `
  --out "out.wav"
```

配布物内のWORLDライブラリとブリッジは自動検出します。イントネーション補正は`0`から`1`で指定できます。

```powershell
.\UtauTTS\utautts-cli.exe `
  --renderer worldline-hybrid `
  --intonation-strength 0.6 `
  --voicebank "sample\uta" `
  --text "こんにちは、今日はいい天気です。" `
  --out "out.wav" `
  --plan-out "plan.json"
```

WORLDを使わない場合は`--renderer waveform`を指定するか、オプションを省略します。

## HTTP API

```powershell
.\UtauTTS-Server\utautts-server.exe `
  --voice-dir "sample" `
  --renderer worldline-hybrid
```

## 開発

```powershell
go test ./...
go run ./cmd/utautts-cli --voicebank "sample/uta" --text "こんにちは。" --out "out.wav"
go run ./cmd/oto-inspect --oto "sample/uta"
```

構成は[docs/architecture.md](docs/architecture.md)、モーラ長の学習は[docs/training.md](docs/training.md)を参照してください。