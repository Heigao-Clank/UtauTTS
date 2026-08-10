# UtauTTS Server

GUIを含まないWindows x64向けHTTPサーバーです。.NET 8ランタイムが必要です。

```powershell
.\utautts-server.exe `
  --voice-dir "voice" `
  --renderer waveform
```

標準では実行ファイルと同じ場所の`voice`ディレクトリを読み込みます。利用可能な音源一覧と再走査は次のAPIで取得・実行できます。

```http
GET /voicebanks
POST /voicebanks/reload
```

```http
POST /synthesize
Content-Type: application/json
```

```json
{
  "text": "こんにちは、今日はいい天気です。",
  "voicebank_id": "足立レイver3.5.0",
  "intonation_strength": 0,
  "apply_pitch": false
}
```

主なオプション：

- `--voice-dir`: ボイスバンクを格納したディレクトリ
- `--renderer`: 初期値と推奨は`waveform`。`waveform-long`、`worldline`、`worldline-hybrid`、`worldline-hybrid-cv`、`worldline-hybrid-cv-balanced`は比較研究用
- `--host`: 待受アドレス。初期値は`127.0.0.1`
- `--port`: ポート。初期値は`8080`
- `--prosody`: 学習済みモーラ長モデル

`intonation_strength`の初期値は`0`、`apply_pitch`の初期値は`false`です。直接ピッチ加工は声質と明瞭度を損なう場合があるため、比較実験でのみ有効にしてください。WORLD系レンダラを使う場合、`worldline.dll`とブリッジは実行ファイルと同じディレクトリから自動検出します。第三者ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。
