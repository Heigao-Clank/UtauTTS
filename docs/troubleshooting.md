# トラブルシューティング

## 音源が一覧に表示されない

- 音源が実行ファイルと同じ階層の `voice/<音源名>/` に置かれているか確認します。
- `oto.ini` が音源フォルダ内にあるか確認します。
- ZIP展開後に `voice/音源名/音源名/oto.ini` のような二重フォルダになっていても、再読込時に1階層内側まで自動検出します。
- 配置後に「ファイル」→「音源を再読込」を実行します。
- 音源に付属する利用条件と、対応している音源形式（CV・VCVなど）を確認します。

## `utautts-openjtalk-features.exe not found` と表示される

Open JTalkの解析helperが見つかっていません。配布版では、実行ファイルの隣にある `runtime` ディレクトリへ次のファイルが含まれます。

```text
runtime/utautts-openjtalk-features.exe
runtime/open_jtalk_dic_utf_8-1.11/
```

ファイルが存在しない場合は、配布ZIPを再展開して確認してください。別のマシンでだけ消える場合は、Windows Defenderなどのセキュリティソフトを確認してください。

## `Open JTalk frontend failed` が表示される

helper、Open JTalk辞書、音源の読み込みに問題がないか確認します。入力文章の読みが特殊な場合は、「設定」→「辞書設定...」で表記と読みを登録してから、文章を再解析してください。

CLIやServerで個別にruntimeの場所を指定する場合は、`--openjtalk-features` と `--openjtalk-dictionary` を使用できます。

## モーラ数や読みが一致しないエラーが表示される

UtauTTS側の読みとOpen JTalk側の解析結果が一致していない可能性があります。

- 固有名詞や略語などを辞書へ登録します。
- 長音、促音、拗音を含む読みを確認します。
- 文章を変更して再解析し、表示された読みとモーラ列を確認します。
- それでも解決しない場合は、エラーログと入力文章を添えて報告してください。

## Rendererやruntime assetが見つからない

まず `waveform` Rendererで合成できるか確認してください。OpenUTAU Classic faithful系を使う場合は、`runtime/worldline.dll` とWorldline bridgeが必要です。CUDA版はCUDA対応の配布物にだけ含まれます。

RendererのIDや必要なassetは [モデル／Rendererプラグイン](plugins.md) と、配布物の `../plugins/renderers/*/plugin.json` で確認できます。

## 音声が生成されない、または合成が遅い

合成中はログウィンドウに処理内容が表示されます。入力文章、音源、Renderer、モデル、runtimeの組み合わせを確認してください。まずは `waveform`、抑揚モデルなし、短い文章で試すと原因を切り分けやすくなります。

音源やモデルを使用する際は、それぞれの配布条件とライセンスを確認してください。UtauTTSは第三者の音源・モデル・runtimeの利用結果について保証しません。
