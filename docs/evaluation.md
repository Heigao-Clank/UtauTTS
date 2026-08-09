# 接続品質の評価

`connection-eval`は合成WAVと、その生成時に`--plan-out`で保存した合成計画を読み、ユニット境界を測定します。各境界の測定時刻は音符開始位置ではなく、実効発声先行と実効オーバーラップから求めたクロスフェード区間の中央です。JSONには区間の開始・終了も記録します。

```powershell
go run ./cmd/utautts-cli `
  --renderer waveform `
  --voicebank sample/uta `
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
