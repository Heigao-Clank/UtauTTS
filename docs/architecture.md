# 構成

UtauTTSは、UTAUボイスバンクに収録された原音を選択・配置・接続し、必要に応じて学習した日本語イントネーションを適用します。ボイスバンク自体や話者の声をニューラルネットワークで生成する方式ではありません。

## 合成パイプライン

1. `frontend`: 入力文章を読みとモーラへ変換します。イントネーションモデルが必要な場合は、配布物内のOpenJTalk frontendがアクセント句、アクセント核、単語境界、品詞を生成します。
2. `voicebank`: `oto.ini` と `prefix.map` から各モーラの原音候補を作成します。原音設定との整合性を表すtarget scoreと、隣接原音の境界を表すjoin scoreを使い、フレーズ全体の経路を選択します。
3. `prosody`: 固定値または自己記述モデルからモーラ長・音量・ピッチを決めます。v8 frame modelは10ms単位の相対ピッチ曲線を出力します。
4. `render`: 合成計画に従って原音を時間配置し、波形を生成します。

## 原音選択

標準の選択方式は `viterbi` です。各位置のtarget scoreと隣接原音のjoin scoreをフレーズ全体で評価します。`greedy` と `target-only` も互換用の選択方式として利用できます。

VCVまたは語頭用alias、CV、ワイルドカードの順に候補の優先度を付けます。`prefix.map` の指定音階aliasを優先し、ひらがな・カタカナの表記差は同じ優先度として扱います。促音に対応する原音がない場合は、無音の `<closure>` を挿入して発話全体の合成を継続します。

## Renderer

RendererのID、表示名、説明、対応機能、必要な資産は `plugins/renderers/*/plugin.json` が管理します。形式と追加方法は [モデル／Rendererプラグイン](plugins.md) を参照してください。

### `waveform`

原音をWSOLAで時間伸縮し、絶対時刻へ配置します。隣接ユニットは相補的にクロスフェードします。外部音声処理依存がなく、同じ入力と設定から同じWAVを生成する、CLIとServerの標準Rendererです。

### `openutau-classic-worldline-faithful`

GUIのv8イントネーションプロファイルで使用するRendererです。OpenUTAU Classicに近いphoneme timing、Worldline resampling、5点envelopeを使い、frame modelの相対ピッチ曲線を原音へ適用します。`worldline.dll` と専用bridgeが必要です。

配布物には比較・診断用のRendererが含まれる場合がありますが、標準構成には使用しません。実験Rendererの互換性や必要資産は各plugin manifestを確認してください。

## GUIとHTTP Server

デスクトップGUIはQt Quick/QMLで構築し、GoバックエンドをC ABI経由で同一プロセスから呼び出します。音源列挙、読み解析、合成にHTTP ServerやWebViewは使用しません。

HTTP Serverは `cmd/utautts-server` と `internal/api` に独立しており、CLIと同じRenderer・モデルcatalogを使用します。詳細は [UtauTTS Server](server.md) を参照してください。

## 配布モデル

モデルJSON自身が `id`、`display_name`、`description`、`recommended_renderers` を持ちます。v0.0.1に同梱するモデルは `frame-intonation-v8` です。GUI、CLI、Serverはモデルの互換Rendererと必要なruntime assetをcatalogから解決します。
