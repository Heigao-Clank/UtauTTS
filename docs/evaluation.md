# 接続品質の評価

`connection-eval`は合成WAVと、その生成時に`--plan-out`で保存した合成計画を読み、ユニット境界を測定します。各境界の測定時刻は音符開始位置ではなく、実効発声先行と実効オーバーラップから求めたクロスフェード区間の中央です。JSONには区間の開始・終了も記録します。

```powershell
go run ./cmd/utautts-cli `
  --renderer waveform `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは。" `
  --out out/waveform.wav `
  --plan-out out/waveform-plan.json

go run ./cmd/connection-eval `
  --wav out/waveform.wav `
  --plan out/waveform-plan.json `
  --out out/waveform-evaluation.json
```

## 指標

- `click`: 境界をまたぐ1サンプルの振幅差。小さいほどクリックが少ない
- `rms_delta_db`: 境界前後20msの音量差
- `spectrum_delta_db`: 境界前後の対数スペクトル差
- `f0_delta_cents`: 両側が有声音の場合のF0差

要約値はポーズを挟まない隣接ユニットだけから計算します。ポーズ後の発話開始点も`boundaries`には残るため、立ち上がりの診断には利用できます。

これらは自然さの完全な代替ではありません。同じ文章・音源・設定でレンダラや実装を比較し、聴感評価で確認する対象を絞るための回帰指標です。

## 原音選択方式の一括比較

`connection-compare`は同じ入力を`target-only`、`greedy`、`viterbi`で合成し、方式ごとのWAV、合成計画、接続評価と`comparison.json`を生成します。

```powershell
go run ./cmd/connection-compare `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは、今日はいい天気です。" `
  --renderer waveform `
  --out out/connection-compare
```

`changed_units_from_target_only`が0なら、その文章と音源ではjoin costによって原音選択が変化していません。処理時間は一度ウォームアップした後の合成全体を測定します。

`--join-model`を指定すると`learned-viterbi`も生成し、`changed_units_from_handcrafted_viterbi`で手設計経路との差を確認できます。モデルを使った計画には各接続の予測確率も保存されます。

## 候補ラティスの監査

`connection-lattice`は採用されなかった候補を含め、target score、直前層から到達できる最良の手設計join score、学習join scoreと確率、各方式の選択候補をJSONへ保存します。

```powershell
go run ./cmd/connection-lattice `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは。" `
  --join-model out/connection-model/model.json `
  --out out/lattice.json
```

## 複数文ベンチマーク

`connection-benchmark`は手設計Viterbiと学習Viterbiを複数文で比較し、原音変更数と接続指標を境界数で重み付け集計します。一つの文章でaliasが不足しても失敗内容をJSONへ残して次の文章を評価します。

```powershell
go run ./cmd/connection-benchmark `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --join-model out/connection-model/model.json `
  --text "こんにちは、今日はいい天気です。" `
  --text "明日も公園へ出かけましょう。" `
  --out out/benchmark.json
```

`--join-scale`でlearned logitの倍率を変更できます。単一指標だけに合わせず、クリック、スペクトル差、RMS差、原音変更数と聴感を併せて決定します。

join costを学習するための境界ペア生成は[接続学習データセット](connection-dataset.md)を参照してください。
