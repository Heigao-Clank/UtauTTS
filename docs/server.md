# UtauTTS Server

GUIを含まないWindows x64向けHTTPサーバーです。.NET 8ランタイムが必要です。

```powershell
.\utautts-server.exe `
  --voice-dir "voice" `
  --renderer waveform
```

標準では実行ファイルと同じ場所の`voice`ディレクトリを読み込みます。利用可能な音源一覧と再走査は次のAPIで取得・実行できます。

```http
GET /api/voicebanks
POST /api/voicebanks/reload
```

```http
POST /api/synthesize
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
- `--renderer`: Renderer plugin ID。省略時はmanifestの`default_priority`が最も高いものを使う
- `--renderer-dir`: Renderer pluginの検索directory。複数回指定できる
- `--model-dir`: 自己記述モデルJSONの検索directory。複数回指定できる
- `--host`: 待受アドレス。初期値は`127.0.0.1`
- `--port`: ポート。初期値は`8080`
- `--prosody`: 学習済み韻律モデル。外部言語特徴が必要なモデルでは同梱OpenJTalk frontendを自動利用する
- `--openjtalk-features` / `--openjtalk-dictionary`: 自動検出を使わずhelperまたは辞書を明示する開発用オプション

`intonation_strength`の初期値は`0`、`apply_pitch`の初期値は`false`です。直接ピッチ加工は声質と明瞭度を損なう場合があるため、比較実験でのみ有効にしてください。WORLD系レンダラを使う場合、`worldline.dll`とブリッジは配布物の`runtime`ディレクトリから自動検出します。旧形式との互換性のため、実行ファイルと同じディレクトリも探索します。第三者ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。

Web UIは`POST /api/synthesize/audio`から`audio/wav`を直接取得します。複数発話は`POST /api/synthesize/batch`で単一ZIPとして取得できます。従来のJSON/base64応答は互換用途として残しています。

`POST /api/voicebanks`による音源パス登録は既定で無効です。必要な場合だけ`--allow-voicebank-registration`を指定でき、登録先は`--voice-dir`以下に制限されます。デスクトップランチャーは起動ごとの認証トークンを自動設定します。独立サーバーでも`--auth-token`を指定できます。
