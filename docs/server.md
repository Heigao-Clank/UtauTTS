# UtauTTS Server

GUIを含まないWindows x64向けHTTPサーバーです。.NET 8ランタイムが必要です。

サーバーは初期状態で `127.0.0.1:8080` のみを待ち受けます。LANや外部から接続できるアドレスで起動する場合は、必ず `--auth-token` を設定してください。

```powershell
.\utautts-server.exe `
  --voice-dir "voice" `
  --renderer waveform
```

標準では実行ファイルと同じ場所の `voice` ディレクトリを読み込みます。音源はフォルダごとに配置し、`voicebank_id` には `/api/voicebanks` で取得したIDを指定します。省略した場合はID順で最初の音源が使われます。

```http
GET /api/health
GET /api/voicebanks
POST /api/voicebanks/reload
GET /api/models
GET /api/renderers
POST /api/analyze
POST /api/synthesize/audio
POST /api/synthesize/batch
```

```http
POST /api/synthesize/audio
Content-Type: application/json
```

```json
{
  "text": "こんにちは、今日はいい天気です。",
  "voicebank_id": "足立レイver3.5.0",
  "model_id": "frame-intonation-v8",
  "renderer": "openutau-classic-worldline-faithful",
  "intonation_strength": 1,
  "apply_pitch": true
}
```

成功時の `/api/synthesize/audio` のレスポンスは `audio/wav` です。レスポンスヘッダー `X-UtauTTS-Reading` に使用した読み、`X-UtauTTS-Engine` にRenderer IDが入ります。モデルやRendererを指定しない場合は、モデルなし・既定Renderer（通常は `waveform`）で合成します。

主なオプション：

- `--voice-dir`: ボイスバンクを格納したディレクトリ
- `--renderer`: Renderer plugin ID。省略時はmanifestの`default_priority`が最も高いものを使う
- `--renderer-dir`: Renderer pluginの検索directory。複数回指定できる
- `--model-dir`: 自己記述モデルJSONの検索directory。複数回指定でき、リクエストの`model_id`で選択する
- `--host`: 待受アドレス。初期値は`127.0.0.1`
- `--port`: ポート。初期値は`8080`
- `--openjtalk-features` / `--openjtalk-dictionary`: 自動検出を使わずhelperまたは辞書を明示する開発用オプション

`intonation_strength`は`0`〜`2`で、初期値は`0`です。`apply_pitch`の初期値は`false`です。自動イントネーションを適用するには、モデル、frame pitch対応Renderer、`apply_pitch: true`を指定します。直接ピッチ加工は声質と明瞭度を損なう場合があるため、結果を確認しながら使用してください。WORLD系Rendererを使う場合、必要assetはRenderer manifestから解決します。第三者ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。

複数発話は`POST /api/synthesize/batch`で単一ZIPとして取得できます。各項目にファイル名と合成リクエストを指定します。

```json
{
  "items": [
    {
      "name": "001.wav",
      "request": {
        "text": "こんにちは",
        "voicebank_id": "足立レイver3.5.0"
      }
    }
  ]
}
```

APIは`/api/*`だけを公開します。`--auth-token`を使用する場合は `Authorization: Bearer <token>` を指定します。

JSON本文は1 MiB、1発話の`text`と`kana`はそれぞれ500文字までです。batchは16発話、展開前WAV合計256 MiBまでに制限されます。未知のJSON fieldは入力ミスとして拒否されます。

`POST /api/voicebanks`による音源パス登録は既定で無効です。必要な場合だけ`--allow-voicebank-registration`を指定でき、登録先は`--voice-dir`以下に制限されます。GUIはHTTPサーバーを使用せず、同梱GUIの音源・辞書設定がServerへ送信されることもありません。

利用できるRendererは `waveform`、`openutau-classic-worldline-faithful`、CUDA対応時の `openutau-classic-worldline-faithful-gpu` です。実際の一覧とモデルの詳細は、それぞれ `/api/renderers` と `/api/models` で確認してください。
