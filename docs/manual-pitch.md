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

メイン画面下部には、選択中の発話に対応するモーラごとのピッチグラフが常に表示されます。点を上下にドラッグすると補正値が直ちに現在の発話へ保存され、ピッチ処理も自動的に有効になります。横軸は読みのモーラ順、縦軸はcent（±300 cent）です。句読点などの休止モーラは編集点として表示されません。

GUIの編集内容は発話ごとに現在のセッション中保持されます。文章を変更すると読みと補正点は破棄され、新しい解析結果に合わせて作り直されます。

JSONで一部のモーラだけを指定した場合、未指定モーラの補正値は0 centです。指定点の値を発話全体へ延長することはありません。

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
