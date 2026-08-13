# モデル／Rendererプラグイン

UtauTTSでは、画面や配布scriptにモデル名とRenderer名を列挙しません。各要素が安定ID、表示名、説明、互換情報を所有し、アプリケーション側の設定は検索directoryと明示的な既定IDだけを扱います。

## Renderer

Rendererは`plugins/renderers/<plugin>/plugin.json`を1つ持ちます。

```json
{
  "manifest_version": 1,
  "kind": "renderer",
  "id": "example.renderer",
  "display_name": "Example Renderer",
  "description": "画面に表示する説明",
  "backend": "waveform",
  "version": "1",
  "experimental": false,
  "default_priority": 0,
  "capabilities": {
    "frame_pitch": false,
    "boundary_bridge": true
  },
  "assets": {}
}
```

`id`は保存データやAPIで使うプラグイン固有ID、`backend`はUtauTTSが実行する実装adapterです。この分離により、同じbackendへ異なる名前・資産・既定値を持つ複数のRendererを追加できます。現在のmanifest APIは組み込みbackendの構成をプラグイン化するもので、任意native codeをプロセスへロードしません。

`default_priority`が最大のものが既定Rendererです。同値の場合は`display_name`順です。`acceleration`には`cpu`、`cuda`、`directml`のいずれかを指定でき、GUIは利用可能な実行デバイスとモデルの`recommended_renderers`を照合して初期Rendererを選びます。CLI/serverの`--renderer`はこれを明示的に上書きします。必要DLLやモデルはmanifestからの相対pathを`assets`へ記述します。

認識するasset key:

- `worldline`
- `worldline_bridge`
- `worldline_r2_mel`
- `worldline_r2_vocoder`

Qt GUI、native backend、HTTP API、CLIは同じcatalogを使用します。追加directoryはnative JSONの`renderer_directories`、CLI/serverの反復可能な`--renderer-dir`で指定できます。

## 抑揚モデル

モデルは別manifestを必要とせず、モデルJSON自身がidentityを持ちます。

```json
{
  "id": "my-model-v1",
  "display_name": "My intonation model",
  "description": "モデルの用途と学習条件",
  "recommended_renderers": ["openutau-classic-worldline-faithful"],
  "default_priority": 100,
  "version": 8,
  "feature_version": 1,
  "mode": "intonation_frame_tcn_accent_bounded"
}
```

`id`と`display_name`のないJSONはモデルpluginとして扱いません。学習scriptは上記fieldを出力します。過去の学習出力を移行する場合は、installerへidentityを明示してJSON自身を書き換えます。

```powershell
.\tools\install-prosody-model.ps1 `
  -ModelPath .\out\prosody\my-model.json `
  -Id my-model-v1 `
  -DisplayName "My intonation model" `
  -RecommendedRenderer openutau-classic-worldline-faithful `
  -DestinationDirectory .\models
```

GUIとserverは`models/`を走査します。追加directoryはnative JSONの`model_directories`、CLI/serverの`--model-dir`で指定します。CLIの`--prosody`にはmodel IDを指定します。

## 配布物

release buildは`plugins/renderers/`と、`models/`に明示的にinstallされた自己記述モデルをGUI・serverの両方へコピーします。研究出力directoryからv8/v9など特定filenameを推測してコピーする処理はありません。`models/`が空ならrelease buildは失敗します。これにより、モデル追加時にQt、Go、release scriptの複数箇所を修正する必要がなくなります。
