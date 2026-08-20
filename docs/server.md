# UtauTTS Server

GUIを含まないWindows x64向けHTTPサーバーです。.NET 8ランタイムが必要です。

サーバーは初期状態で `127.0.0.1:8080` のみを待ち受けます。LANや外部から接続できるアドレスで起動する場合は、必ず `--auth-token` を設定してください。

```powershell
.\utautts-server.exe `
  --voice-dir "voice" `
  --renderer waveform
```

標準では実行ファイルと同じ場所の `voice` ディレクトリを読み込みます。音源はフォルダごとに配置し、`voicebank_id` には `/api/voicebanks` で取得したIDを指定します。省略した場合はID順で最初の音源が使われます。

起動すると `UTAUTTS_READY=http://127.0.0.1:8080` の形式で待受URLを標準出力へ書き出します。

## コンソールUI

ブラウザで `http://127.0.0.1:8080/` を開くと、`/api/*` を直接呼び出せる簡易クライアントと、エンドポイント・制限の簡単なドキュメントが見れるhtmlを返します

- 稼働状況・音源・モデル・Rendererの一覧表示
- 文章の解析（`/api/analyze`）と、読み・モーラ列の表示
- 文章・音源・モデル・Renderer・duration等を指定した合成（`/api/synthesize/audio`）。結果の再生とWAVダウンロード、使用した読み・engineヘッダーの表示
- `--auth-token` 使用時は、ページ内のトークン入力に保存すると以降のAPI呼び出しへ `Authorization: Bearer <token>` を付加します

コンソールUI（`/` と `/ui`）は公開されます。認証・Origin検査は `/api/*` にのみ適用されます。

## エンドポイント一覧

| Method | Path | 説明 |
|---|---|---|
| `GET` | `/api/health` | 稼働確認 |
| `GET` | `/api/voicebanks` | 音源一覧 |
| `POST` | `/api/voicebanks` | 音源の登録（既定で無効） |
| `POST` | `/api/voicebanks/reload` | 音源ディレクトリの再読込 |
| `GET` | `/api/models` | 抑揚モデル一覧 |
| `GET` | `/api/renderers` | Renderer一覧 |
| `POST` | `/api/analyze` | 文章から読み・モーラ列への変換 |
| `POST` | `/api/synthesize/audio` | 単一発話の合成（WAV） |
| `POST` | `/api/synthesize/batch` | 複数発話の合成（ZIP） |

## 共通仕様

- エラーは `{"error":"説明"}` のJSONで、対応するHTTPステータスコードとともに返ります。
- JSON本文は1 MiBまでです。未知のJSON fieldは入力ミスとして拒否されます（400）。
- ボディは単一のJSONオブジェクトでなければなりません。
- 1発話の `text` と `kana` はそれぞれ500文字までです（413）。
- `manual_pitch` のpointsは最大1000個です（413）。
- batchは16発話、展開前WAV合計256 MiBまでです（413）。
- 合成は最大4並行に制限されます。超過分は空きを待って順に処理されます。

### 認証

`--auth-token` を設定すると、全エンドポイントで `Authorization: Bearer <token>` ヘッダーが必要になります。無い場合は401を返します。トークンは定数時間比較で検証されます。

GET以外のリクエストに `Origin` ヘッダーがあり、それが待受ホストのorigin（`http://<host>` / `https://<host>`）と一致しない場合は403で拒否されます。

非ループバックアドレス（例 `0.0.0.0`）で認証トークンなしに起動すると、起動時に警告が出力されます。

## 各エンドポイント

### `GET /api/health`

```json
{"status":"ok","engine":"waveform"}
```

`engine` には既定RendererのIDが入ります。

### `GET /api/voicebanks`

ID順にソートされた音源一覧です。

```json
{
  "voicebanks": [
    {
      "id": "足立レイver3.5.0",
      "name": "足立レイ",
      "path": "C:\\...\\voice\\足立レイver3.5.0",
      "oto_file_count": 12,
      "phoneme_count": 2640,
      "diagnostic_count": 3,
      "alias_counts": {"CV": 1200, "VCV": 1400, "VC": 40, "other": 0},
      "vcv_contexts": {"-": 50, "a": 280, "i": 270},
      "has_initial_vcv": true,
      "has_n_context_vcv": true
    }
  ]
}
```

`id` は `voicebank_id` に指定する値で、音源フォルダ名です。`phoneme_count` は全 `oto.ini` のエントリ数、`diagnostic_count` はoto.iniの診断で問題があるエントリ数です。
`alias_counts` と `vcv_contexts` は音源のalias能力を表す診断情報で、実際の各モーラではaliasの存在と`oto.ini`設定が最終的な選択を決めます。

### `POST /api/voicebanks`

音源パスを動的に登録します。既定では無効で、`--allow-voicebank-registration` を指定した場合のみ使えます。登録先は `--voice-dir` 以下に制限され、シンボリックリンク解決後に範囲外なら400で拒否されます。

```json
{"name": "My Bank", "path": "voice/my-bank"}
```

成功すると登録された音源オブジェクト（`GET /api/voicebanks` と同じ形式）を返します。`name` は省略できます（省略時は音源の表示名）。

### `POST /api/voicebanks/reload`

`--voice-dir` を再走査し、音源一覧を置き換えます。レスポンスは `GET /api/voicebanks` と同じ形式です。失敗時は500です。

### `GET /api/models`

利用可能な抑揚モデルの一覧です。

```json
{
  "models": [
    {
      "id": "frame-intonation-v8",
      "display_name": "Frame intonation TCN v8",
      "description": "JSUT frame-level learned intonation model",
      "path": "C:\\...\\models\\frame-intonation-v8.json",
      "version": 8,
      "mode": "intonation_frame_tcn_accent_bounded",
      "outputs": {"pitch": true},
      "recommended_renderers": ["openutau-classic-worldline-faithful-gpu", "openutau-classic-worldline-faithful"],
      "default_priority": 100,
      "requires_features": true,
      "frame_contour": true
    }
  ]
}
```

`model_id` には `id` を指定します。

### `GET /api/renderers`

```json
{
  "default_renderer": "openutau-classic-worldline-faithful",
  "renderers": [
    {"id": "waveform", "display_name": "Waveform", "description": "...", "backend": "waveform", "capabilities": {"frame_pitch": false}, "default_priority": 0}
  ]
}
```

`default_renderer` はサーバー起動時の既定Rendererです。

### `POST /api/analyze`

文章を読みとモーラ列へ変換します。合成前の読み確認に使います。

```json
{"text": "こんにちは"}
```

```json
{
  "reading": "コンニチハ",
  "morae": [
    {"position": 0, "mora": "コ", "pause": false},
    {"position": 1, "mora": "ン", "pause": false},
    {"position": 2, "mora": "ニ", "pause": false},
    {"position": 3, "mora": "チ", "pause": false},
    {"position": 4, "mora": "ハ", "pause": false}
  ]
}
```

`pause` が `true` のモーラは休止です。`text` が空なら400、500文字超なら413、変換に失敗すると422です。

### `POST /api/synthesize/audio`

単一発話を合成し、`audio/wav` を返します。

```json
{
  "text": "こんにちは、今日はいい天気です。",
  "voicebank_id": "足立レイver3.5.0",
  "model_id": "frame-intonation-v8",
  "renderer": "openutau-classic-worldline-faithful",
  "alias_policy": "auto",
  "intonation_strength": 1,
  "apply_pitch": true
}
```

レスポンスヘッダー `X-UtauTTS-Reading` に使用した読み、`X-UtauTTS-Engine` にRenderer IDが入ります。

リクエストフィールド：

| field | 型 | 既定値 | 説明 |
|---|---|---|---|
| `text` | string | | 日本語文章。`kana` とどちらか一方が必須 |
| `kana` | string | | 読み仮名の直接指定 |
| `voicebank_id` | string | ID順先頭 | `GET /api/voicebanks` の `id` |
| `tone` | string | `C4` | `prefix.map` 使用時の音階 |
| `model_id` | string | なし | `GET /api/models` の `id` |
| `renderer` | string | 既定Renderer | `GET /api/renderers` の `id` |
| `alias_policy` | string | `auto` | `auto`（VCV優先・CVへ局所fallback）、`vcv-prefer`（VCVをより強く優先）、`cv-only`（単独音のみ） |
| `mora_duration_ms` | number | `140` | 基本モーラ長（0〜1000） |
| `pause_duration_ms` | number | `180` | 句読点の休止長（0〜3000） |
| `mora_durations_ms` | number[] | | モーラごとの長さ。値は0〜1000 |
| `intonation_strength` | number | `0` | 音源ピッチ安定化と句曲線の強さ（0〜2） |
| `apply_pitch` | boolean | `false` | 波形ピッチ再サンプリング |
| `manual_pitch` | object | なし | 手動ピッチ編集（[manual-pitch.md](manual-pitch.md) のJSON） |

ステータスコード：

- `200`: WAVバイナリ。`X-UtauTTS-Engine` / `X-UtauTTS-Reading` ヘッダー付き
- `400`: `text`/`kana` の両方なし、範囲外のduration・`intonation_strength`、音源・モデル・Rendererが未登録（`ErrUnavailable`）
- `413`: 文字数・`manual_pitch` points超過、JSON 1 MiB超過
- `422`: 合成の失敗（読み変換失敗、モデル評価失敗、未知の`alias_policy`など）

### `POST /api/synthesize/batch`

複数発話を単一のZIPとして取得します。レスポンスは `application/zip` で、`Content-Disposition: attachment; filename="utautts-audio.zip"` が付きます。

```json
{
  "items": [
    {
      "name": "001.wav",
      "request": {"text": "こんにちは", "voicebank_id": "足立レイver3.5.0"}
    },
    {
      "name": "002.wav",
      "request": {"kana": "オハヨウ", "voicebank_id": "足立レイver3.5.0"}
    }
  ]
}
```

`name` はパス部分が除去され、空や `..` の場合は `utterance-N.wav` に置き換えられます。重複するファイル名は400です。途中のアイテムで合成に失敗すると、そのエラーが `item N: <error>` 形式で返ります（後続は合成されません）。

## 起動オプション

- `--voice-dir`: ボイスバンクを格納したディレクトリ
- `--renderer`: Renderer plugin ID。省略時はmanifestの`default_priority`が最も高いものを使う
- `--renderer-dir`: Renderer pluginの検索directory。複数回指定できる
- `--model-dir`: 自己記述モデルJSONの検索directory。複数回指定でき、リクエストの`model_id`で選択する
- `--host`: 待受アドレス。初期値は`127.0.0.1`
- `--port`: ポート。初期値は`8080`
- `--auth-token`: 認証トークン。設定すると`Authorization: Bearer <token>`が必須になる
- `--allow-voicebank-registration`: `POST /api/voicebanks` による音源パス登録を許可する（登録先は`--voice-dir`以下に制限）
- `--worldline` / `--worldline-bridge`: worldlineライブラリとbridge実行ファイルを明示する
- `--openjtalk-features` / `--openjtalk-dictionary`: 自動検出を使わずhelperまたは辞書を明示する開発用オプション

## 注意事項

`intonation_strength`は`0`〜`2`で、初期値は`0`です。`apply_pitch`の初期値は`false`です。自動イントネーションを適用するには、モデル、frame pitch対応Renderer、`apply_pitch: true`を指定します。直接ピッチ加工は声質と明瞭度を損なう場合があるため、結果を確認しながら使用してください。WORLD系Rendererを使う場合、必要assetはRenderer manifestから解決します。第三者ライセンスは`THIRD_PARTY_NOTICES.txt`を参照してください。

APIは`/api/*`だけを公開し、コンソールUI（`/`）のみ公開です。GUIはHTTPサーバーを使用せず、同梱GUIの音源・辞書設定がServerへ送信されることもありません。

利用できるRendererは `waveform`、`openutau-classic-worldline-faithful`、CUDA対応時の `openutau-classic-worldline-faithful-gpu` です。実際の一覧とモデルの詳細は、それぞれ `/api/renderers` と `/api/models` で確認してください。
