# UtauTTS Server

GUIを含まないWindows x64向けHTTPサーバーです。.NET 8ランタイムが必要です。

```powershell
.\utautts-server.exe `
  --voice-dir "voice" `
  --renderer worldline-hybrid
```

起動後に<http://127.0.0.1:8080>を開きます。

```http
POST /synthesize
Content-Type: application/json
```

```json
{
  "text": "こんにちは、今日はいい天気です。",
  "voicebank_id": "uta",
  "intonation_strength": 0.6
}
```

主なオプション：

- `--voice-dir`: ボイスバンクを格納したディレクトリ
- `--renderer`: `worldline-hybrid`または`waveform`
- `--host`: 待受アドレス。初期値は`127.0.0.1`
- `--port`: 待受ポート。初期値は`8080`
- `--prosody`: 学習済みモーラ長モデル

`worldline.dll`とブリッジは実行ファイルと同じディレクトリから自動検出します。第三者ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。
