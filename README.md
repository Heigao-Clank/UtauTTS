# UtauTTS

UTAUボイスバンクの原音接続と、深層学習による日本語イントネーションを組み合わせたTTS

UTAUボイスバンクに収録された原音を選び、自然につなぎ、日本語の文章を音声化するTTSソフトウェアです。声そのものをニューラルネットワークで生成するのではなく、ボイスバンクの声質と発音を保ちながら、原音選択・時間配置・接続・イントネーションを組み合わせます。

> **注意:** ボイスバンクを使用する前に、必ず各ボイスバンクに付属する利用規約・ガイドラインを確認してください。UtauTTSおよびその作者は、ボイスバンクの利用に関して生じた問題について責任を負いません。

| 用途 | 構成 | 特徴 |
|---|---|---|
| 明瞭度・原音確認 | `waveform` | 原音のピッチを保った決定的な波形接続。CLI／Serverの既定Renderer |
| GUIの標準プロファイル | `frame-intonation-v8` + `openutau-classic-worldline-faithful` | OpenJTalkのアクセント特徴から10ms単位の相対ピッチを予測し、OpenUTAU Classicに近いtiming・5点envelope・Worldline resamplingで適用 |

v8モデルは音声波形や話者の声を生成しません。必要なモデル・Renderer・runtime assetが揃わない場合は、GUIでエラーの内容を確認してから `waveform` を明瞭度の基準にしてください。`worldline`、`worldline-hybrid*`、`waveform-long`、CUDA系は比較・診断用の実験Rendererです。

対応する音源形式は CV・VCV、複数の `oto.ini`、UTF-8・Shift_JIS、`prefix.map` です。CVVCのVC挿入は未対応です。

## ダウンロード

[リリース](https://github.com/yh2237/UtauTTS/releases) から最新版をダウンロードしてください。

- `UtauTTS-win-x64.zip`: GUI、CLI、診断・学習ツール、runtime、モデル、Renderer、同梱音源
- `UtauTTS-Server-win-x64.zip`: GUIを含まないHTTPサーバー

### GUI

`UtauTTS-win-x64.zip` を展開し、`utautts.exe` を実行してください。

追加のUTAU音源は、実行ファイルと同じ階層の `voice` ディレクトリへ、音源ごとにフォルダを分けて配置してください。「再読込」で一覧へ反映できます。GUI版には足立レイ UTAU音源 ver3.5.0 を初期音源として同梱しています。利用条件は [docs/voicebank.md](docs/voicebank.md) と音源内の文書を確認してください。

### CLI

同梱音源を使う最小例です。`--renderer waveform` は、原音の明瞭度を確認する安全な基準です。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --renderer waveform `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。" `
  --out ".\out.wav"
```

v8モデルを明示する場合は、次のようにモデルIDと互換Rendererを指定します。配布物内のOpenJTalk frontendが、任意文章の読みとアクセント特徴を実行時に生成します。

```powershell
.\UtauTTS\tools\utautts-cli.exe `
  --voicebank ".\UtauTTS\voice\足立レイver3.5.0" `
  --renderer openutau-classic-worldline-faithful `
  --prosody frame-intonation-v8 `
  --prosody-pitch-only `
  --out ".\out.wav" `
  --text "あらゆる現実をすべて自分のほうへねじ曲げたのだ。"
```

モデルやRendererの追加・互換性は [docs/plugins.md](docs/plugins.md)、ピッチ編集は [docs/manual-pitch.md](docs/manual-pitch.md)、辞書設定は [docs/dictionary.md](docs/dictionary.md) を参照してください。

### HTTP Server

`UtauTTS-Server-win-x64.zip` を展開し、`voice` ディレクトリへ音源を置いて起動します。

```powershell
.\UtauTTS-Server\utautts-server.exe `
  --voice-dir ".\UtauTTS-Server\voice" `
  --renderer waveform
```

APIの詳細、認証、入力制限は [docs/server.md](docs/server.md) を参照してください。サーバーは初期状態で `127.0.0.1:8080` のみを待ち受けます。外部から接続できるアドレスを指定する場合は認証トークンを設定してください。

## ビルド

Windows x64でのリリースビルドには、Go、Qt 6.5以降（Qt Quick・Qt Multimedia・Qt Concurrent）、CMake、Ninja、MSYS2 Clang、.NET 8 SDK、Python 3.12 x64、インターネット接続が必要です。Qt SDKを `.qt/<version>/mingw_64` に配置すると自動検出します。それ以外の場所ではcompiler kitのパスを `QT_ROOT` に設定してください。

```powershell
.\build.bat
```

引数を省略するとWindows版をビルドします。`.\build.bat linux` はWSL経由、`.\build.bat both` はWindows版とLinux版を順にビルドします。ビルド時にはOpenUtau由来の依存ファイルを取得し、SHA-256を検証します。第三者ライセンスと音源の利用条件は [THIRD_PARTY_NOTICES.txt](./THIRD_PARTY_NOTICES.txt)、`licenses/`、[docs/voicebank.md](docs/voicebank.md) を確認してください。

開発時のテストは次のとおりです。

```powershell
go test ./...
```

## Special Thanks

- Testing by [アアアアアアア（@a7_riri）](https://x.com/a7_riri)

## ライセンス

UtauTTS本体は [MIT License](./LICENSE) です。同梱ボイスバンク、OpenUtau由来ファイル等には個別の利用条件があるため、配布物の [THIRD_PARTY_NOTICES.txt](./THIRD_PARTY_NOTICES.txt) と各同梱文書を必ず確認してください。
