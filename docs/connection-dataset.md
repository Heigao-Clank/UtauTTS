# 接続学習データセット

`connection-dataset`はUTAUボイスバンクから、join cost学習用の弱教師付き境界ペアをJSONLで生成します。同じWAV内でoffset順に隣り合う原音を正例、右側を同じ発音末尾の別録音原音へ差し替えたものを負例とします。標準の`hard`戦略は、手設計上も滑らかで簡単には区別できない負例を優先します。旧方式との比較には`--negative-strategy rotating`を使います。

```powershell
go run ./cmd/connection-dataset `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --out out/connection-dataset/adachi-rei.jsonl `
  --negatives 3 `
  --negative-strategy hard
```

`--limit`で正例数を制限できます。`<out>.report.json`には正負例数、負例候補を作れなかった正例数、境界フレームを抽出できなかった正負例数を分けて出力します。

各レコードには次が入ります。

- `label`: 自然な同一録音接続は1、差し替え接続は0
- `record_id`: 人手ラベルと照合する安定ID
- `label_source`: `weak_recording_continuity`または`human`
- `weight`: 学習時の例別重み。省略時は1
- `group_id`: 元の自然接続単位。同じIDの正例と負例は必ず同じデータ分割へ入れる
- `previous` / `current`: alias、WAV、oto.ini行、offsetなどの由来
- `features`: 両側30msの正規化対数スペクトル、RMS、F0、その差とピッチ同期波形相関
- `handcrafted_score`: 現行join scoreとの比較用。学習入力には使わない

`features`からは、弱教師ラベルを直接表す「同じWAVか」「録音順か」を意図的に除外しています。ファイルパスや`provenance`、`handcrafted_score`もラベルを推測できるため、モデル入力には使用しません。

## 学習・評価時の分割

これは聴感評価ではなく、連続録音を自然とみなした弱教師です。次の条件を守って評価します。

- 正例とそこから作った負例を`group_id`単位で分割する
- 同じボイスバンクをtrainとvalidationへ跨がせない
- 最終評価はleave-one-voicebank-outとAB/ABX聴感評価で行う
- `valid: false`を含むレコードは、欠損値として明示的に扱うか学習対象から除く
- 小規模・単独音音源では互換aliasの別録音候補がなく、負例を生成できない場合がある

このJSONLは接続モデルの入力形式を固定する第一段階です。positiveが常に自然、negativeが常に不自然とは限らないため、後続段階では少数の人手比較ラベルで校正します。

## ベースライン学習

一つの音源内で挙動を確認する場合は、`group_id`単位で分割します。

```powershell
go run ./cmd/connection-train `
  --dataset out/connection-dataset/adachi-rei.jsonl `
  --out out/connection-model/adachi-rei.json `
  --validation-fraction 0.2
```

複数音源から汎用的な接続規則を学ぶ場合は、`--dataset`を繰り返し指定し、検証音源を`--validation-voicebank`で丸ごと隔離します。足立レイ単体で動作を確認する場合は、group単位の分割を使用します。

```powershell
go run ./cmd/connection-train `
  --dataset out/connection-dataset/adachi-rei.jsonl `
  --validation-fraction 0.2 `
  --out out/connection-model/adachi-rei.json
```

モデルは差分スペクトル10帯域、スペクトル平均差、RMS差、F0差、voicing不一致、波形相関を入力します。波形は240点へ正規化し、±5msの位相ずれを探索した最大相関を使います。標準は正規化ロジスティック回帰です。診断用に`--model mlp --hidden 32`で1隠れ層MLPも学習でき、同じGo推論経路から利用できます。モデルJSONにはモデル種別、seed、正規化係数、重み、学習音源、validationのaccuracy、balanced accuracy、AUC、log lossを保存します。`<out>.split.json`で分割後・欠損除外後の件数と音源名を監査できます。モデルv1/v2も推論互換のため読み込めます。

```powershell
go run ./cmd/connection-train `
  --dataset out/connection-dataset/adachi-rei.jsonl `
  --model mlp `
  --hidden 32 `
  --seed 1 `
  --out out/connection-model/adachi-rei-mlp.json
```

## 人手ラベル

アノテーションJSONの`record_id`に対応する例を、人手ラベルへ置き換えられます。`label`は自然なら1、不自然なら0です。人手ラベルは標準で弱教師の3倍の重みになります。

```json
{"version":1,"annotations":[{"record_id":"...","label":1}]}
```

```powershell
go run ./cmd/connection-label-import `
  --dataset out/connection-dataset/adachi-rei.jsonl `
  --annotations responses/connection-labels.json `
  --out out/connection-dataset/adachi-rei-human.jsonl
```

## 合成への適用

```powershell
go run ./cmd/utautts-cli `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは。" `
  --join-model out/connection-model/adachi-rei.json `
  --plan-out out/learned-plan.json `
  --out out/learned.wav
```

学習確率はclipped logitで−8から＋8の音響join scoreへ変換します。同一録音を順方向に使う場合の＋8は別に維持します。合成計画v7の`join_cost_mode`、`join_model_version`、各原音の`join_probability`と`join_score`から判断を監査できます。

標準のlogit倍率は4です。`--join-scale`で変更できますが、倍率を上げるほど良くなるとは限りません。未知文を複数用意し、`connection-benchmark`で手設計方式に対する改善と悪化を両方確認します。

`connection-compare`へ同じモデルを渡すと、`target-only`、`greedy`、手設計Viterbi、学習Viterbiを一括生成します。

```powershell
go run ./cmd/connection-compare `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは、今日はいい天気です。" `
  --join-model out/connection-model/adachi-rei.json `
  --out out/connection-compare/learned
```
