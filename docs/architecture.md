# 構成

UtauTTSはUTAUボイスバンクに収録された原音を選んで配置・接続し、必要に応じて学習した日本語イントネーションを加えます。ボイスバンク自体や話者の声をニューラルネットワークで生成する方式ではありません。

内部データ構造、各Rendererの処理、研究結果から決めたことは[技術設計ガイド](technical-design.md)に書いてあります。

## 合成パイプライン

1. `frontend`: 入力文章を読みとモーラへ変換します。イントネーションモデルを使う場合は配布物内のOpenJTalk frontendがアクセント句、アクセント核、単語境界、品詞を作ります。
2. `voicebank`: `oto.ini`と`prefix.map`から各モーラの原音候補を作ります。原音設定との合い方を表すtarget scoreと隣接原音の境界を表すjoin scoreを使い、フレーズ全体の経路を選びます。
3. `prosody`: 固定値か自己記述モデルからモーラ長・音量・ピッチを決めます。`frame-intonation-v8`は10ms単位の相対ピッチ曲線、`prosody-multitask-v1`はそれに加えてモーラ長も出力します。

`prosody-multitask-v1`の`mora_duration` headはモーラ単位の特徴（`bias`/`position`/`from_end`/`mora`/`prev`/`next`/アクセント系）から対数スケールのモーラ長倍率を予測します。予測は`plan`のモーラ長へ`DurationFactor`として反映されて実際の原音配置と接続に使われます。GUIは同じ値を`mora_durations_ms`として表示しますが編集時は手動指定値（`MoraDurationsMS`）が優先です。`--prosody-pitch-only`を指定するとモーラ長・音量の予測を無効にしてピッチ予測だけを使います。
4. `render`: 合成計画に従って原音を時間配置して波形を作ります。

## 原音選択

標準の選択方式は`viterbi`です。各位置のtarget scoreと隣接原音のjoin scoreをフレーズ全体で評価します。`greedy`と`target-only`も互換用の選択方式として利用できます。

VCV、VC+CV（CVVC）、CV、ワイルドカードの候補をモーラ単位の経路へまとめます。CVVCのVCは単独のモーラではなく次のCVへ入る遷移ユニットとして扱います。`AliasPolicy`の`auto`（既定）は音源全体のVC/VCV収録比を見て`legacy`相当か`cvvc-enhanced`相当を選びます。`legacy`はv0.0.9互換、`cvvc-enhanced`はCVVC優先・sequential timing・VC遷移音量35%の一括指定です。細かく指定したい場合は`vcv-prefer`、`cvvc-prefer`、`cv-only`も使えます。`prefix.map`の指定音階aliasを優先してひらがな・カタカナの表記差は同じ優先度として扱います。促音に対応する原音がなければ無音の`<closure>`を挿入して発話全体の合成を続けます。Planには要求された方針と解決後の方針、alias種別、遷移ユニット、fallback tierを記録します。

## Renderer

RendererのID、表示名、説明、対応機能、必要な資産は`plugins/renderers/*/plugin.json`で管理します。形式と追加方法は[モデル／Rendererプラグイン](plugins.md)にあります。

### `waveform`

原音をWSOLAで時間伸縮して絶対時刻へ配置し、隣接ユニットを相補的にクロスフェードします。外部音声処理依存がなく同じ入力と設定から同じWAVを作るRendererです。原音の明瞭度を確認する比較基準にも使えます。frame modelの相対ピッチ曲線が与えられた場合は`resampleForPitchCurve`による可変レート線形リサンプリングで各原音へ時間変化するピッチを適用します。モーラ単位の固定係数と同じく係数は0.75〜1.35に制限されます。

### `openutau-worldline-r-faithful`

GUI、CLI、Serverの既定Rendererです。OpenUTAU 0.1.565の`PhraseSynth` APIを使い各原音のWORLD特徴を共通の時間軸へ配置してからフレーズ全体を合成します。`worldline.dll`と専用bridgeが必要です。

### `openutau-classic-worldline-faithful`

OpenUTAU Classicに近いphoneme timing、Worldline resampling、5点envelopeを使ってframe modelの相対ピッチ曲線を原音へ適用します。`worldline.dll`と専用bridgeが必要です。

### `openutau-classic-worldline-faithful-gpu`

CPU版faithful Rendererと同じ処理をCUDAで実行する任意Rendererです。CUDA対応の配布物でのみ利用できます。利用できない環境ではCPU版を選択してください。

現在の配布対象は`openutau-worldline-r-faithful`、`waveform`、Classic faithful、条件付きのClassic faithful CUDAです。古い実験Rendererや研究中の方式は入れていません。

## GUIとHTTP Server

デスクトップGUIはQt Quick/QMLで作りGoバックエンドをC ABI経由で同一プロセスから呼び出します。音源列挙、読み解析、合成にHTTP ServerやWebViewは使いません。

HTTP Serverは`cmd/utautts-server`と`internal/api`に分かれていてCLIと同じRenderer・モデルcatalogを使います。詳細は[UtauTTS Server](server.md)にあります。
