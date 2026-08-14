# 同梱ボイスバンク

GUI版には、メカニカルガール公式配布の「足立レイ UTAU音源 ver3.5.0」を初期ボイスバンクとして同梱します。リポジトリでは公式ZIPを`voice/足立レイver3.5.0.zip`に保持し、リリースビルド時に`voice/足立レイver3.5.0`へ展開します。

GUIは`character.txt`の`name`と`image`を読み、画像を発話行と音源選択欄へ表示します。発話行の画像はドラッグによる並べ替えハンドルとして機能します。UTF-8、Shift_JIS、BOM付きUTF-16LE/BEを判別し、`image`が音源ディレクトリ外を指す場合は読み込みません。

## 利用条件

足立レイ音源はUtauTTS本体のMIT Licenseの対象外です。音源に同梱されたreadme、ガイドライン、開発履歴などが適用されます。リリース生成時もこれらのファイルを削除・変更しません。

- [公式配布ページ](https://mechanicalgirl.jp/adachi-rei/)
- [キャラクター／音源使用ガイドライン](https://mechanicalgirl.jp/guidelines/)

## SHA-256

同梱している公式ZIPのSHA-256

```text
B96D1B21145F22E573AFD9EC8AEAAD0EC9CBAEE581C2623C64ADDEB31DE46B3D
```

## 開発時のパス

`build.bat`を実行すると、開発用コマンドから次のパスで同梱音源を参照できます。

```text
release/UtauTTS/voice/足立レイver3.5.0
```

配布ZIP内からCLIを実行する場合は次のパスです。

```text
.\UtauTTS\voice\足立レイver3.5.0
```
