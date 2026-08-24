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

壊れたmanifest、未対応backend、Renderer IDの重複は起動時エラーになります。有効な項目だけを表示して問題を黙って無視することはありません。モデルJSONの破損とモデルIDの重複も同様です。`id` と `display_name` を持たないJSONはモデルpluginとして扱われません。

`default_priority`が最大のものが既定Rendererです。同値の場合は`display_name`順です。`acceleration`には`cpu`または`cuda`を指定でき、GUIは利用可能な実行デバイスとモデルの`recommended_renderers`を照合して初期Rendererを選びます。CLI/serverの`--renderer`はこれを明示的に上書きします。必要DLLやモデルはmanifestからの相対pathを`assets`へ記述します。

認識するasset key:

- `worldline`
- `worldline_bridge`

現在の配布物に含まれるRenderer IDは次のとおりです。

- `waveform`: CPUで動作する標準の波形接続
- `openutau-classic-worldline-faithful`: CPU版faithful Renderer
- `openutau-classic-worldline-faithful-gpu`: CUDA版faithful Renderer。CUDA対応の配布物にのみ含まれます。

Qt GUI、native backend、HTTP API、CLIは同じcatalogを使用します。追加directoryはnative JSONの`renderer_directories`、CLI/serverの反復可能な`--renderer-dir`で指定できます。

## 抑揚モデル

モデルは別manifestを必要とせず、モデルJSON自身がidentityを持ちます。モデルIDはGUI、CLI、Serverで共通です。

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

GUIで調整した抑揚からversion 11の個人補正モデルを作る手順は、[手動調整から抑揚モデルを作る](prosody-model-training.md)を参照してください。

配布モデルは次の2種類です。

- `frame-intonation-v8`: Open JTalkのアクセント特徴を使ったフレーム単位のイントネーション
- `prosody-multitask-v1`: v8系のイントネーションに加えたモーラ長予測

どちらもfaithful系Rendererを推奨します。`waveform`もframe pitchに対応しますが、可変レートresamplingとWSOLAで適用するため、原音確認ではpitchを無効にした出力も比較基準にしてください。

## 配布物

release buildは`plugins/renderers/`と、`models/`に明示的にinstallされた自己記述モデルをGUI・serverの両方へコピーします。研究出力directoryから特定filenameを推測してコピーする処理はありません。`models/`が空ならrelease buildは失敗します。これにより、モデル追加時にQt、Go、release scriptの複数箇所を修正する必要がなくなります。
