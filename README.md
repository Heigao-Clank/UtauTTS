# UtauTTS

UTAUボイスバンクを音源ごと再学習せずに利用する、日本語向けTTSの実験プロジェクトです。

現在は最初の決定論的ベースラインを実装しています。日本語文を辞書発音へ変換し、CV／VCVの原音を選択して、`oto.ini` のタイミングに従って時間伸縮・オーバーラップ合成します。同じ入力と設定からは常に同じWAVを生成します。

## 現在できること

- サブディレクトリを含む複数の `oto.ini` を一音源として読み込む
- UTF-8／Shift_JISの原音設定を読む
- CV／VCVを前の母音に応じて選択する
- `character.txt` と `prefix.map` を読み、多音階音源の収録セットを選択する
- モーラ長を大幅に超えるVCV先行発声を自動圧縮し、多重発音を抑える
- ひらがな・カタカナの読みをモーラに分割する
- 漢字かな混じり文を純GoのKagome IPA辞書で発音へ変換する
- Pythonや学習済みモデルなしでWAVを生成する
- HTTP APIとブラウザ上の簡易UIを使う

## CLI

```powershell
go run ./cmd/utautts --voicebank "sample/uta" --text "こんにちは、今日はいい天気です。" --out "out.wav"
```

自然音声から、固定ベースラインに対するモーラ長の局所的な補正だけを学習するラインも利用できます。発話全体の速度を正規化し、F0・音量・ポーズは音源と固定規則を維持します。

```powershell
go run ./cmd/prosody-dataset --jsut data/jsut/basic5000 --out out/prosody/jsut.jsonl
go run ./cmd/prosody-train --dataset out/prosody/jsut.jsonl --out out/prosody/model.json
go run ./cmd/utautts --voicebank "sample/uta" --text "こんにちは。" --prosody out/prosody/model.json --out "out.wav"
```

データ形式、評価指標、弱アラインメントの制約は [docs/training.md](docs/training.md) を参照してください。

多音階音源では `--tone G4` のように収録音高を指定できます。指定音高が存在しない場合は `prefix.map` 内の最も近い音高を選択します。

使用した原音と配置時間を確認する場合：

```powershell
go run ./cmd/utautts --voicebank "sample/uta" --text "こんにちは、今日はいい天気です。" --out "out.wav" --plan-out "plan.json"
```

音源の検査：

```powershell
go run ./cmd/oto-inspect --oto "sample/uta"
go run ./cmd/oto-inspect --oto "sample/uta" --kana "こんにちは"
```

## HTTPサーバー

```powershell
go run ./cmd/utautts-server --voice-dir sample
```

その後、<http://127.0.0.1:8080> を開きます。

`POST /synthesize` の例：

```json
{
  "text": "こんにちは、今日はいい天気です。",
  "voicebank_id": "uta"
}
```

## 現在の制限

- 未知語・英数字の読み上げ規則は未実装
- 学習モデルを指定しない場合、モーラ長も固定規則
- 現在の学習データは弱アラインメントで、アクセント句や長距離文脈は未対応
- 学習補正は発話内のモーラ長だけで、F0・音量・ポーズは変更しない
- CVVCのVCユニット挿入は未実装
- 時間伸縮と接続品質はベースライン段階
- 長いVCVタイミングの自動正規化はヒューリスティックで、すべての音源に最適とは限らない

設計方針は [docs/architecture.md](docs/architecture.md)、今後の作業は [docs/roadmap.md](docs/roadmap.md) を参照してください。
