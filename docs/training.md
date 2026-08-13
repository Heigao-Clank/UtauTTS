# モーラ長の学習

この学習ラインは固定規則に対する局所的なモーラ長、F0、エネルギーの残差を学習する研究経路です。コーパス話者の声質や絶対音高、発話全体の速度・音量は転写せず、各発話の中央値で正規化します。

現在の標準合成は、明瞭度を優先して`waveform`とイントネーション強度`0`を使います。学習F0の直接適用はリサンプル由来のケロケロ感を生むため、モデルの客観評価またはブラインド比較に限定します。今後は予測F0を原音候補のtarget costへ使う方向を優先します。

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

### TCN学習のGPU利用

PyTorch版のモーラTCNとフレームTCNは、既定の`--device auto`でCUDA、Intel XPU、Apple MPSの順に利用可能なアクセラレータを選び、利用できない環境ではCPUへ戻ります。GPU対応版PyTorchが必要です。明示する場合は`--device cuda`、複数GPUでは`--device cuda:1`、CPUで再現確認する場合は`--device cpu`を指定します。

```powershell
python tools/train-frame-intonation-tcn.py `
  --dataset out/prosody/jsut-5000-hts.jsonl `
  --out out/prosody/frame-intonation.json `
  --device cuda `
  --batch-size 32
```

音声の読み込み・F0抽出・Open JTalk整列はCPU、TCNのforward/backwardと検証推論はGPUで実行します。選択されたデバイスは標準出力とモデルJSONの`training.device`へ記録されます。短いデータセットでは転送コストが勝つため、`--device cpu`のほうが速い場合があります。

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
  --system-a-renderer waveform `
  --system-b-renderer waveform `
  --system-b-prosody out/prosody/model.json `
  --corpus docs/evaluation-corpus.json `
  --out out/listening/prosody
```

## v9 / v9.1句アンカー補正モデル

v9はOpen JTalkのアクセント基準曲線に対する、句頭・アクセント核・句末・疑問上昇の補正アンカーを学習します。既存のfaithful rendererへ滑らかなフレーム曲線として渡すため、rendererを追加する必要はありません。v9.1では、Open JTalkのモーラ整列を厳格に検証し、WORLDのオクターブ誤推定を補正してから平滑化log-F0残差を教師にします。

```powershell
python tools/train-intonation-v9.py `
  --dataset out/prosody/jsut-5000-hts.jsonl `
  --out out/prosody/intonation-phrase-anchor-v9.json `
  --worldline .tmp-openutau-reference/worldline.dll
```

v9.1を学習する場合は、次の出力名を使います。Open JTalkが必須で、整列率が全体の既定値60%未満の場合は失敗します。整列できなかったレコードはfallback化せず、学習対象から除外します。

```powershell
python tools/train-intonation-v9.py `
  --dataset out/prosody/jsut-5000-hts.jsonl `
  --out out/prosody/intonation-phrase-anchor-v9-1.json `
  --worldline .tmp-openutau-reference/worldline.dll
```

v9.1の採用判断はアンカーMAEだけでなく、アクセント核の高低差、句末上昇、曲率、波形基準からの歯抜け・音質劣化を同じ評価コーパスで確認します。`--no-openjtalk-accent`はfallback検証専用です。

この例では主にdurationとenergyの差を比較します。F0だけを比較するときは`--system-b-prosody-pitch-only`を加え、必要に応じてWORLD系レンダラと`--intonation-strength`を明示してください。
