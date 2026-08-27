# UtauTTS

UTAUボイスバンクの原音接続に学習ベースのイントネーション調整を加えた日本語TTS

UTAUボイスバンクに収録された原音を選んで配置し、必要に応じて学習したイントネーションやモーラ長を加えます。声そのものをニューラルネットワークで生成するのではなく、ボイスバンクに収録されている声を使う方式です。

> ボイスバンクを使う前に各ボイスバンクの利用規約・ガイドラインを確認してください。UtauTTSおよびその作者はボイスバンクの利用で生じた問題について責任を負いません。

## はじめる

[リリース](https://github.com/yh2237/UtauTTS/releases)から環境に合うZIPをダウンロードしてください。

| パッケージ | 内容 |
| --- | --- |
| `UtauTTS-win-x64.zip` | Windows x64向けGUI、CLI、ツール |
| `UtauTTS-linux-x64.zip` | Linux x64向けGUI、CLI、ツール |
| `UtauTTS-Server-win-x64.zip` | Windows x64向けHTTPサーバー |
| `UtauTTS-Server-linux-x64.zip` | Linux x64向けHTTPサーバー |

Windows版はZIPを展開して`utautts.exe`を実行します。Linux版にはQt 6.5以降と日本語フォントが必要です。詳しい導入手順とボイスバンクの追加方法は[インストール](docs/installation.md)を参照してください。

操作方法は[GUIの使い方](docs/gui.md)、初期値やTXT／LAB書き出しは[設定](docs/settings.md)にまとめています。起動や合成に失敗する場合は[トラブルシューティング](docs/troubleshooting.md)を確認してください。

## CLIの最小例

`waveform` Rendererで同梱音源からWAVを書き出す例です。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --renderer waveform `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "こんにちは、今日はいい天気です。" `
  --out ".\out.wav"
```

モデルを使う例や全オプションは[コマンドライン（CLI）](docs/cli.md)を参照してください。

## HTTP Server

Server版を展開して起動すると`http://127.0.0.1:8080/`でコンソールUIを利用できます。

```powershell
.\UtauTTS-Server\utautts-server.exe --voice-dir ".\UtauTTS-Server\voice"
```

API、認証、入力制限は[UtauTTS Server](docs/server.md)を参照してください。

## 仕組みと対応範囲

| 用途 | 構成 | 特徴 |
| --- | --- | --- |
| 明瞭度・原音確認 | `waveform` | 原音をWSOLAで伸縮し、波形を直接接続。frameピッチ曲線も可変レートリサンプリングで適用可能 |
| 標準構成 | `frame-intonation-v8` + `openutau-worldline-r-faithful` | OpenJTalkのアクセント特徴から10ms単位の相対ピッチを予測し、OpenUTAU WORLDLINE-Rのフレーズ合成で適用 |
| 自動モーラ長 | `prosody-multitask-v1` + faithful系Renderer | v8系のイントネーションに加えて、モーラ長を予測して合成タイミングへ適用し、結果をGUIへ表示。`--prosody-pitch-only`でモーラ長予測を無効化 |

モデルは音声波形や話者の声を生成しません。必要なモデル・Renderer・runtime assetが揃わない場合はGUIでエラーを確認し、`waveform`を明瞭度の基準にしてください。既定Rendererは`openutau-worldline-r-faithful`です。Classic faithfulと対応するWindows配布物ならClassic faithful CUDAも選べます。

CV・VCV・CVVC、複数の`oto.ini`、UTF-8・Shift_JIS、`prefix.map`に対応します。CVVCは試験対応で利用できない境界ではVCVまたはCVへ局所的にfallbackします。`presamp.ini`の完全互換や音源固有のalias変換には対応していません。

## ドキュメント

- [インストール](docs/installation.md)
- [GUIの使い方](docs/gui.md)
- [設定](docs/settings.md)
- [コマンドライン（CLI）](docs/cli.md)
- [UtauTTS Server](docs/server.md)
- [トラブルシューティング](docs/troubleshooting.md)
- [UtauTTSはどうやって音声を作るのか](docs/how-utautts-speaks.md)
- [ドキュメント一覧](docs/README.md)

ソースからのビルドは[開発環境とビルド](docs/building.md)を参照してください。
内部実装や研究上の設計判断は[技術設計ガイド](docs/technical-design.md)にまとめています。

## 謝辞

- Testing by [アアアアアアア（@a7_riri）](https://x.com/a7_riri)

## ライセンス

UtauTTSのソースコードは[MIT License](./LICENSE)です。`models/`の学習済みモデルはCC BY-SA 4.0です。同梱ボイスバンク、学習済みモデル、OpenUtau由来ファイルなどには個別の利用条件があります。配布物の[THIRD_PARTY_NOTICES.txt](./THIRD_PARTY_NOTICES.txt)、`licenses/`、各同梱文書を確認してください。
