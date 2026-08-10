# UtauTTS Server

GUIを含まないWindows x64向けHTTPサーバーです。.NET 8ランタイムが必要です。

```powershell
.\utautts-server.exe `
  --voice-dir "voice" `
  --renderer worldline-hybrid-cv-balanced
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
  "intonation_strength": 0.6
}
```

主なオプション：

- `--voice-dir`: ボイスバンクを格納したディレクトリ
- `--renderer`: 足立レイでの推奨は`worldline-hybrid-cv-balanced`。比較用の`worldline-hybrid-cv`、従来の`worldline-hybrid`、`waveform`、実験的な`waveform-long`も指定可能
- `--host`: 待受アドレス。初期値は`127.0.0.1`
- `--port`: ポート。初期値は`8080`
- `--prosody`: 学習済みモーラ長モデル

`worldline.dll`とブリッジは実行ファイルと同じディレクトリから自動検出します。第三者ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。
