# 構成

UtauTTSは、UTAUボイスバンクに収録された原音を選択・配置・接続し、必要に応じて学習した日本語イントネーションを適用します。ボイスバンク自体や話者の声をニューラルネットワークで生成する方式ではありません。

## 合成パイプライン

1. `frontend`: 入力文章を読みとモーラへ変換します。イントネーションモデルが必要な場合は、配布物内のOpenJTalk frontendがアクセント句、アクセント核、単語境界、品詞を生成します。
2. `voicebank`: `oto.ini` と `prefix.map` から各モーラの原音候補を作成します。原音設定との整合性を表すtarget scoreと、隣接原音の境界を表すjoin scoreを使い、フレーズ全体の経路を選択します。
3. `prosody`: 固定値または自己記述モデルからモーラ長・音量・ピッチを決めます。`frame-intonation-v8` は10ms単位の相対ピッチ曲線を、`prosody-multitask-v1` はそれに加えてモーラ長を出力します。

`prosody-multitask-v1` の `mora_duration` headはモーラ単位の特徴（`bias`/`position`/`from_end`/`mora`/`prev`/`next`/アクセント系）から対数スケールのモーラ長倍率を予測します。予測は `plan` のモーラ長へ `DurationFactor` として反映され、実際の合成タイミング（原音配置・接続）に使われます。GUIは同じ値を `mora_durations_ms` として表示しますが、表示は結果の確認用であり、編集時は手動指定値（`MoraDurationsMS`）が優先されます。`--prosody-pitch-only` を指定するとモーラ長・音量の予測を無効化し、ピッチ予測のみを使用します。
4. `render`: 合成計画に従って原音を時間配置し、波形を生成します。

## 原音選択

標準の選択方式は `viterbi` です。各位置のtarget scoreと隣接原音のjoin scoreをフレーズ全体で評価します。`greedy` と `target-only` も互換用の選択方式として利用できます。

VCVまたは語頭用alias、CV、ワイルドカードの順に候補の優先度を付けます。`prefix.map` の指定音階aliasを優先し、ひらがな・カタカナの表記差は同じ優先度として扱います。促音に対応する原音がない場合は、無音の `<closure>` を挿入して発話全体の合成を継続します。

## Renderer

RendererのID、表示名、説明、対応機能、必要な資産は `plugins/renderers/*/plugin.json` が管理します。形式と追加方法は [モデル／Rendererプラグイン](plugins.md) を参照してください。

### `waveform`

原音をWSOLAで時間伸縮し、絶対時刻へ配置します。隣接ユニットは相補的にクロスフェードします。外部音声処理依存がなく、同じ入力と設定から同じWAVを生成する、CLIとServerの標準Rendererです。frame modelの相対ピッチ曲線が与えられた場合は、`resampleForPitchCurve` による可変レート線形リサンプリングで各原音へ時間変化するピッチを適用します（モーラ単位の固定係数と同様に、係数は0.75〜1.35に制限されます）。

### `openutau-classic-worldline-faithful`

GUIのイントネーションプロファイルで使用するRendererです。OpenUTAU Classicに近いphoneme timing、Worldline resampling、5点envelopeを使い、frame modelの相対ピッチ曲線を原音へ適用します。`worldline.dll` と専用bridgeが必要です。

### `openutau-classic-worldline-faithful-gpu`

CPU版faithful Rendererと同じ処理をCUDAで実行する任意Rendererです。CUDA対応の配布物でのみ利用できます。利用できない環境ではCPU版を選択してください。

現在の配布対象は `waveform`、CPU版faithful、条件付きのGPU版faithfulです。古い実験Rendererや研究中の方式は配布対象ではありません。

## GUIとHTTP Server

デスクトップGUIはQt Quick/QMLで構築し、GoバックエンドをC ABI経由で同一プロセスから呼び出します。音源列挙、読み解析、合成にHTTP ServerやWebViewは使用しません。

HTTP Serverは `cmd/utautts-server` と `internal/api` に独立しており、CLIと同じRenderer・モデルcatalogを使用します。詳細は [UtauTTS Server](server.md) を参照してください。
