# イントネーションとモーラ長の編集

UtauTTSではモデルが予測したイントネーションとモーラ長を合成前に確認し、必要なところだけ手動で調整できます。手動ピッチ編集は読みの各モーラに対する音高補正（cent）を指定するJSON形式です。補正は学習イントネーションへ加算されて選択したRendererのピッチ処理へ渡されます。CLIの`--prosody`にはモデルファイル名ではなくインストール済みモデルの`id`を指定します。

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

`position`は読みのモーラ配列に対する0始まりの位置です。`mora`も書いた場合はその位置のモーラと一致するか検証されます。休止モーラは編集対象になりません。

`mode`は次の2種類です。

- `offset`: 学習イントネーションへ補正値を加算します。通常はこちらを使います。
- `replace`: 学習イントネーションを使わず、手動カーブだけを使います。

## GUI

メイン画面下部には選択中の発話に対応するモーラごとのグラフが表示されます。点を上下へドラッグすると補正値が現在の発話へ保存されてピッチ処理も自動で有効になります。横軸は読みのモーラ位置、縦軸はcent（±300 cent）です。句読点などの休止モーラは編集点には表示されません。

モーラの位置にある縦線を左右へドラッグするとモーラ境界を動かして長さを調整できます。`Shift`を押しながら動かすとその位置より後ろの縦線も同じ距離だけ移動します。グラフ下部のモーラ長表示をダブルクリックするとそのモーラと両隣のモーラを基本長へ戻します。

自動モーラ長は`prosody-multitask-v1`を選んだときに表示されます。手動で縦線を動かした発話ではその手動値が自動値より優先されます。`waveform`もモデルのピッチ曲線を適用できますが声質を保ちやすい標準構成は`openutau-worldline-r-faithful`です。

GUIの編集内容は発話ごとに保持されて`.utautts`プロジェクトにも保存されます。文章を変更すると読みと補正点は破棄されて新しい解析結果に合わせて作り直されます。

JSONで一部のモーラだけを指定した場合は未指定モーラの補正値が0 centになります。指定点の値が発話全体へ勝手に延長されることはありません。

## CLI

```powershell
go run ./cmd/utautts-cli `
  --voicebank "release/UtauTTS/voice/ボイスバンク" `
  --text "こんにちは" `
  --renderer openutau-worldline-r-faithful `
  --prosody frame-intonation-v8 `
  --prosody-pitch-only `
  --apply-pitch `
  --manual-pitch "out/manual-pitch.json" `
  --out "out/manual-pitch.wav"
```

手動カーブは10ms刻みへ補間されて急激な変化はRenderer側の安全制約で滑らかに制限されます。モーラ長は`mora_durations_ms`へ読みのモーラ順で入れます。位置も明示する場合は`mora_positions_ms`を使います。
