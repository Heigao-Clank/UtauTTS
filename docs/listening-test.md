# AB・ABX聴感評価

`listening-test`は二つのレンダラをランダムにA/Bへ割り当て、ブラウザだけで回答できるブラインド評価一式を生成します。標準では`waveform`と実験的な`waveform-long`を比較します。

## AB自然さ評価

```powershell
go run ./cmd/listening-test `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --join-model out/connection-model/model.json `
  --mode ab `
  --text "あけかこか" `
  --text "明日も公園へ出かけましょう。" `
  --out out/listening/long-unit-ab
```

`public/index.html`を開くと、Aが自然、Bが自然、同程度から回答できます。二方式のWAVが完全一致する文章は自動的に除外されます。`public`ディレクトリだけを評価者へ渡し、方式名を含む`answer-key.json`は評価終了まで非公開にしてください。

固定評価セットを使う場合は、繰り返しの`--text`の代わりに`--corpus docs/evaluation-corpus.json`を指定します。manifestにはコーパス名、case ID、乱数seed、非公開keyにはモデルパスと変更原音の位置・alias・スコアも記録されます。公開ファイルから方式や原音差分は分かりません。

手設計join costと学習join costを同じrendererで比較する場合は、システム別にモデルを指定します。

```powershell
go run ./cmd/listening-test `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --system-a-renderer waveform `
  --system-b-renderer waveform `
  --system-b-join-model out/connection-model/model.json `
  --mode ab `
  --text "明日も公園へ出かけましょう。" `
  --out out/listening/learned-selection
```

`--join-model`は両システム共通、`--system-a-join-model`と`--system-b-join-model`は個別指定です。

## ABX識別評価

```powershell
go run ./cmd/listening-test `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --join-model out/connection-model/model.json `
  --mode abx `
  --text "あけかこか" `
  --out out/listening/long-unit-abx
```

ABXではXがA/Bのどちらと同一かを回答します。ABは自然さの選好、ABXは二方式を聴き分けられるかの確認であり、目的が異なります。音量を固定し、ヘッドホン利用、試行順のランダム化、複数評価者を推奨します。

## 回答の集計

ブラウザが保存した`listening-results.json`を、非公開のanswer keyと照合します。`--response`は評価者ごとに繰り返せます。

```powershell
go run ./cmd/listening-score `
  --key out/listening/long-unit-ab/answer-key.json `
  --response responses/person-01.json `
  --response responses/person-02.json `
  --out out/listening/long-unit-ab/score.json
```

ABでは方式別選好数、同率、同率を除いた選好率と95% Wilson信頼区間を出します。ABXでは正答・誤答・不明、不明を除いた正答率と95% Wilson信頼区間を集計します。`cases`にはcase ID、文章、回答、対応する方式を残すため、変更原音との突き合わせに使えます。少数試行の差を結論とせず、音源・文章・評価者を増やしてください。

## waveform-long

`waveform-long`は、次の全条件を満たす隣接原音だけを一つの連続区間として時間伸縮します。

- 同じWAV・同じoto.ini
- oto.ini行が連続
- offsetが増加
- 合成上のモーラ位置が連続
- pitch/energy係数が同じ

条件を満たさない部分は通常の`waveform`へ戻ります。合成計画の`long_unit_group`と`long_unit_size`で適用箇所を確認できます。現在は実験方式であり、初回の連続4原音では通常方式より客観接続指標が悪化したため、聴感評価なしに標準へ切り替えないでください。
