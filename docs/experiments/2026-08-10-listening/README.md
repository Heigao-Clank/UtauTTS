# 接続モデル聴感評価 2026-08-10

固定コーパス`utautts-ja-core-v1`と重音テトOU用日本語統合ライブラリーを使い、waveformレンダラ上の原音選択だけをブラインドAB比較した。評価者は1名、乱数seedは`20260810`。完全一致した音声は試行から除外した。

## 結果

| 比較 | 試行 | 左側の選好 | 右側の選好 | 同程度 |
| --- | ---: | ---: | ---: | ---: |
| 手設計 vs Logistic、join scale 4 | 7 | 手設計 3 | Logistic 0 | 4 |
| Logistic vs MLP、join scale 4 | 1 | Logistic 0 | MLP 1 | 0 |
| 手設計 vs Logistic、join scale 2 | 6 | 手設計 1 | Logistic 0 | 5 |

scale 4で手設計が選ばれた文章:

- `greeting`: こんにちは、今日はいい天気です。
- `voiceless`: 小さな靴を机に置いた。
- `mixed`: 明日も公園へ出かけましょう。

LogisticとMLPで差があったのは`fricative`の「涼しい風が砂浜を吹き抜ける。」だけで、MLPが選ばれた。この位置ではMLPが複雑な新候補を選んだのではなく、手設計joinが変更した候補からtarget優先候補へ戻していた。

scale 2では原音変更数が9から6へ減った。識別できたのは`greeting`だけで、scale 4と同じく手設計が選ばれた。残り5試行は評価者には違いを識別できなかった。

## 客観指標

固定12文、181原音での集計値:

| 方式 | 変更原音 | mean click | mean spectrum delta |
| --- | ---: | ---: | ---: |
| 手設計 | 0 | 0.02332 | 8.496 dB |
| Logistic、scale 4 | 9 | 0.02241 | 8.126 dB |
| MLP、scale 4 | 10 | 0.02247 | 8.128 dB |
| Logistic、scale 2 | 6 | 0.02302 | 8.085 dB |

客観指標は学習モデルで改善したが、今回の聴感選好とは一致しなかった。現段階では学習モデルを標準化せず、加工後境界を使う特徴、知覚差なしを表すtie、少数の人手比較ラベルを扱う必要がある。

この結果は評価者1名かつ有効選好数が少ないため、一般的な優劣の結論ではなく次の実験設計の根拠として扱う。

## 生回答

- `responses/handcrafted-vs-logistic-scale4.json`
- `responses/logistic-vs-mlp-scale4.json`
- `responses/handcrafted-vs-logistic-scale2.json`
