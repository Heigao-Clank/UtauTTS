# コマンドライン（CLI）

`utautts-cli` は、ボイスバンク・モデル・Rendererを指定して単一のWAVを生成するコマンドライン合成ツールです。GUIやHTTPサーバーと同じ合成パイプラインを使用します。

配布物ではWindowsの `tools/utautts-cli.exe`、Linuxの `tools/utautts-cli` にあります。開発時は `go run ./cmd/utautts-cli` でも実行できます。

## 必須の引数

- `--voicebank <dir>`: UTAUボイスバンクのディレクトリ
- `--out <path>`: 出力WAVのパス
- `--text <文>`, または `--kana <読み>`: 合成する文章。片方だけ指定します（`--kana` は読み仮名を直接指定する場合用）

これらが無い場合は usage とエラーを表示して終了します。`--text` はOpen JTalk frontendによる読み・アクセント解析が必要な場合に使い、`--kana` は読みが既に分かっている場合に使います。

## 基本例

同梱音源で、原音の明瞭度を確認する標準Rendererを使う最小例です。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --renderer waveform `
  --out ".\out.wav"
```

学習イントネーションを適用する例です。モデルIDとframe pitch対応Rendererを指定します。配布物内のOpenJTalk frontendが実行時に読みとアクセント特徴を生成します。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --renderer openutau-classic-worldline-faithful `
  --prosody frame-intonation-v8 `
  --prosody-pitch-only `
  --out ".\out.wav"
```

モーラ長も予測したい場合は `--prosody prosody-multitask-v1` を使います。`--plan-out` で合成計画（各ユニットの配置・タイミング）をJSONへ保存できます。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "こんにちは" `
  --renderer openutau-classic-worldline-faithful `
  --prosody prosody-multitask-v1 `
  --plan-out ".\out\plan.json" `
  --out ".\out.wav"
```

## オプション

| オプション | 既定値 | 説明 |
|---|---|---|
| `--version` | | アプリケーションのバージョンを表示して終了 |
| `--voicebank <dir>` | | ボイスバンクのディレクトリ（必須） |
| `--oto <dir>` | | `--voicebank` の旧名（deprecated） |
| `--text <文>` | | 合成する日本語文章 |
| `--kana <読み>` | | 合成する読み仮名 |
| `--tone` | `C4` | `prefix.map` 使用時に使う音階 |
| `--out <path>` | | 出力WAVのパス（必須） |
| `--plan-out <path>` | | 合成計画JSONを保存するパス |
| `--mora-ms` | `140` | 基本モーラ長（ms） |
| `--pause-ms` | `180` | 句読点の休止長（ms） |
| `--release-ms` | `20` | ユニット末尾のrelease envelope（ms） |
| `--prosody <id>` | | 抑揚モデルのplugin ID |
| `--prosody-pitch-only` | `false` | 学習ピッチのみ適用し、モーラ長・音量は固定値を使う |
| `--manual-pitch <path>` | | 手動ピッチ編集JSON（[manual-pitch.md](manual-pitch.md)） |
| `--prosody-features <path>` | | ケース別のモーラ単位アクセント特徴JSON |
| `--prosody-feature-case <id>` | | `--prosody-features` 内のケースID |
| `--pitch-contours <path>` | | ケース別ピッチ係数JSON（計画へ記録。波形処理には `--apply-pitch` が必要） |
| `--pitch-case <id>` | | `--pitch-contours` 内のケースID |
| `--apply-pitch` | `false` | 波形のピッチ再サンプリング（実験的） |
| `--intonation-strength` | `0` | 音源ピッチ安定化と句曲線の強さ（0〜2） |
| `--renderer <id>` | 既定Renderer | Renderer plugin ID（省略時はmanifestの `default_priority` が最大のもの） |
| `--worldline <path>` | 実行ファイルの隣 | OpenUtau worldlineライブラリ |
| `--worldline-bridge <path>` | | `utautts-worldline-bridge` 実行ファイル |
| `--boundary-bridge-ms` | `0` | 位相を揃えた波形接続修復の最大幅（0で無効） |
| `--boundary-bridge-threshold` | `0` | handcrafted join scoreがこの値以下のとき接続修復を適用 |
| `--selection` | `viterbi` | 原音選択方式（`viterbi`、`greedy`、`target-only`） |
| `--alias-policy` | `auto` | 音源適応モード。`auto`はVC/VCV収録比から自動選択、`legacy`はv0.0.9互換、`cvvc-enhanced`はCVVC優先・sequential timing・VC音量35%。詳細指定として`vcv-prefer`、`cvvc-prefer`、`cv-only`も利用可能 |
| `--join-model <path>` | | 学習済みjoin-costモデルJSON |
| `--join-scale` | `0` | 学習済みlogitスコアの倍率（既定はモデル値または4） |
| `--renderer-dir <dir>` | | Renderer pluginの検索directory（繰り返し指定可） |
| `--model-dir <dir>` | | モデルJSONの検索directory（繰り返し指定可） |
| `--openjtalk-features <path>` | runtime | Open JTalk feature helper（自動検出を上書き） |
| `--openjtalk-dictionary <path>` | runtime | Open JTalk辞書ディレクトリ（自動検出を上書き） |

## モデルとRendererの指定

`--prosody` と `--renderer` にはファイルパスではなく、プラグインの `id` を指定します。idと一覧は [plugins.md](plugins.md) を参照してください。配布物に含まれるのは次のとおりです。

- モデル: `frame-intonation-v8`、`prosody-multitask-v1`
- Renderer: `waveform`、`openutau-classic-worldline-faithful`、CUDA対応時は `openutau-classic-worldline-faithful-gpu`

モデルやRendererは既定のディレクトリ（実行ファイルの隣の `models/`、`plugins/renderers/`）を自動検出します。追加directoryは `--model-dir` / `--renderer-dir` で指定できます。

`--apply-pitch` と `--intonation-strength` による直接的なピッチ加工は、声質と明瞭度を損なう場合があります。自然なイントネーションを試す場合は、モデルとfaithful系Rendererを組み合わせてください。

## 出力

成功するとWAVを書き出し、次の行を表示します。

```
wrote out.wav (4.81s, 44100 Hz, 34 units)
```

合成の失敗や引数エラーは `log.Fatal` で終了コード1を返します。モデル・Rendererが見つからない場合も同様です。原音の明瞭度を先に確認したい場合は `--renderer waveform` を使うのが安全です。
