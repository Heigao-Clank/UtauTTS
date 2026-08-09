# モーラ長の学習

この学習ラインは固定規則に対する局所的なモーラ長補正だけを学習します。コーパス話者の声質、発話全体の速度、F0、音量、ポーズは転写しません。

## データセット作成

JSUT `basic5000`を`data/jsut/basic5000`へ配置します。`transcript_utf8.txt`と`wav/*.wav`が必要です。

```powershell
go run ./cmd/prosody-dataset `
  --jsut data/jsut/basic5000 `
  --out out/prosody/jsut.jsonl
```

JSONLには読み、発話範囲、モーラ境界、長さ、F0、音量を保存します。不採用データと理由は`<出力名>.report.json`へ記録します。`--limit 100`で件数、`--workers N`で並列数を制限できます。

JSUTには音素時刻がないため、現在は予想時刻・音量谷・波形相関を使った弱アラインメントです。境界は確認可能な中間データであり、正確な音素ラベルではありません。

## 学習

```powershell
go run ./cmd/prosody-train `
  --dataset out/prosody/jsut.jsonl `
  --out out/prosody/model.json `
  --epochs 30
```

発話単位で学習・検証を分離し、発話速度を正規化した対数モーラ長の残差を学習します。弱アラインメントの外れ値はHuber勾配で抑えます。

## 使用

```powershell
go run ./cmd/utautts-cli `
  --voicebank sample/uta `
  --text "こんにちは。" `
  --prosody out/prosody/model.json `
  --out out.wav
```

モデル適用時も補正倍率は発話内中央値1.0へ揃え、`0.8`から`1.25`へ制限します。`--prosody`を省略すると固定規則を使います。
