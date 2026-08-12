# 手動イントネーション

手動ピッチ編集は、読みの各モーラに対する音高補正（cent）を指定する JSON 形式です。補正は v8/v9 などの学習イントネーションに加算され、OpenUTAU Classic faithful などのピッチ処理へ渡されます。

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

メイン画面の「イントネーション編集」を押すと、モーラごとのピッチグラフが開きます。青い点を上下にドラッグすると補正値を変更できます。横軸は読みのモーラ順、縦軸は cent（±300 cent）です。「リセット」で全点を 0 に戻し、「適用して閉じる」で現在の編集を次回生成へ反映します。

GUI の編集内容は現在のセッション中に保持されます。別の文章を編集するときは、読みが一致する場合だけ前回の補正が再利用されます。

## CLI

```powershell
go run ./cmd/utautts-cli `
  --voicebank "release/UtauTTS/voice/ボイスバンク" `
  --text "こんにちは" `
  --renderer openutau-classic-worldline-faithful `
  --prosody "models/frame-intonation-v8.json" `
  --prosody-pitch-only `
  --manual-pitch "out/manual-pitch.json" `
  --out "out/manual-pitch.wav"
```

手動カーブは 10ms 刻みへ補間され、急激な変化は renderer 側の安全制約で滑らかに制限されます。
