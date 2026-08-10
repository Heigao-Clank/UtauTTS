# モーラ長の学習

この学習ラインは固定規則に対する局所的なモーラ長、F0、エネルギーの残差を学習します。コーパス話者の声質や絶対音高、発話全体の速度・音量は転写せず、各発話の中央値で正規化します。

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

発話単位で学習・検証を分離し、発話速度を正規化した対数モーラ長、発話中央値に対するF0比とエネルギー比を学習します。弱アラインメントの外れ値はHuber勾配で抑えます。推論時は中央値を1へ戻し、長さを0.8〜1.25、F0を0.97〜1.03、エネルギーを0.9〜1.1へ制限します。

レポートには学習モデルと固定値ベースラインのduration MAE、pitch MAE、energy MAEを併記します。学習値が固定値を上回る場合はモデルを採用しません。

## 使用

```powershell
go run ./cmd/utautts-cli `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは。" `
  --prosody out/prosody/model.json `
  --out out.wav
```

モデル適用時も補正倍率は発話内中央値1.0へ揃え、`0.8`から`1.25`へ制限します。`--prosody`を省略すると固定規則を使います。

聴感比較では片側だけにモデルを指定できます。

```powershell
go run ./cmd/listening-test `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --system-a-renderer worldline-hybrid `
  --system-b-renderer worldline-hybrid `
  --system-b-prosody out/prosody/model.json `
  --corpus docs/evaluation-corpus.json `
  --intonation-strength 0.6 `
  --worldline release/UtauTTS/worldline.dll `
  --worldline-bridge release/UtauTTS/utautts-worldline-bridge.exe `
  --out out/listening/prosody
```
