# イントネーションとモーラ長の編集

UtauTTSでは、モデルが予測したイントネーションとモーラ長を合成前に確認し、必要な部分だけ手動で調整できます。手動ピッチ編集は、読みの各モーラに対する音高補正（cent）を指定するJSON形式です。補正は学習イントネーションに加算され、OpenUTAU Classic faithful系などのピッチ処理へ渡されます。CLIの `--prosody` にはモデルファイル名ではなく、インストール済みモデルの `id` を指定します。

```json
{
  "version": 1,
  "reading": "こんにちは",
  "mode": "offset",
  "points": [
    {"position": 0, "mora": "こ", "cents": 0},
    {"position": 1, "mora": "ん", "cents": 40},
    {"position": 2, "mora": "に", "cents": 80},
    {"position": 3, "mora": "ち", "cents": 20},
    {"position": 4, "mora": "は", "cents": -30}
  ]
}
```

`position` は読みのモーラ配列に対する 0 始まりの位置です。`mora` を指定した場合は、その位置のモーラと一致するか検証されます。休止モーラは編集対象になりません。

`mode` は次の 2 種類です。

- `offset`: 学習イントネーションへ補正値を加算します。通常はこちらを使います。
- `replace`: 学習イントネーションを使わず、手動カーブだけを使います。

## GUI

メイン画面下部には、選択中の発話に対応するモーラごとのグラフが表示されます。点を上下にドラッグすると補正値が現在の発話へ保存され、ピッチ処理も自動的に有効になります。横軸は読みのモーラ位置、縦軸はcent（±300 cent）です。句読点などの休止モーラは編集点として表示されません。

モーラの位置にある縦線を左右へドラッグすると、モーラ境界を動かして長さを調整できます。`Shift`を押しながら動かすと、その位置より後ろの縦線も同じ距離だけ移動します。グラフ下部のモーラ長表示をダブルクリックすると、そのモーラと両隣のモーラを基本長へ戻します。

自動モーラ長は `prosody-multitask-v1` を選択したときに表示されます。手動で縦線を動かした発話では、その手動値が自動値より優先されます。`waveform` Rendererではモデルのピッチ処理を使わないため、イントネーションを聴感へ反映する場合は対応するfaithful系Rendererを選んでください。

GUIの編集内容は発話ごとに現在のセッション中保持されます。文章を変更すると読みと補正点は破棄され、新しい解析結果に合わせて作り直されます。

JSONで一部のモーラだけを指定した場合、未指定モーラの補正値は0 centです。指定点の値を発話全体へ延長することはありません。

## CLI

```powershell
go run ./cmd/utautts-cli `
  --voicebank "release/UtauTTS/voice/ボイスバンク" `
  --text "こんにちは" `
  --renderer openutau-classic-worldline-faithful `
  --prosody frame-intonation-v8 `
  --prosody-pitch-only `
  --apply-pitch `
  --manual-pitch "out/manual-pitch.json" `
  --out "out/manual-pitch.wav"
```

手動カーブは10ms刻みへ補間され、急激な変化はRenderer側の安全制約で滑らかに制限されます。モーラ長は `mora_durations_ms` に値を入れ、読みのモーラ順で指定します。位置を明示する場合は `mora_positions_ms` を使用します。
