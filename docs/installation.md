# インストール

## パッケージを選ぶ

[GitHub Releases](https://github.com/yh2237/UtauTTS/releases)から環境と用途に合うZIPをダウンロードします。

| パッケージ | 用途 |
| --- | --- |
| `UtauTTS-win-x64.zip` | Windows x64でGUIを使う。CLIと補助ツールも同梱 |
| `UtauTTS-linux-x64.zip` | Linux x64でGUIを使う。CLIと補助ツールも同梱 |
| `UtauTTS-Server-win-x64.zip` | Windows x64でHTTP APIを使う |
| `UtauTTS-Server-linux-x64.zip` | Linux x64でHTTP APIを使う |

配布ZIPには合成に必要なモデルとruntimeが含まれます。Worldline bridge用の.NETランタイムやPythonを別途インストールする必要はありません。

## Windows

1. `UtauTTS-win-x64.zip`を任意のフォルダへ展開します。
2. 展開先の`utautts.exe`を実行します。
3. 左側へ文章を入力して再生ボタンで合成を確認します。

ZIP内のファイルは同じ階層構造のまま使用してください。runtimeやモデルだけを移動すると合成できません。

## Linux

Linux GUI版にはQt 6.5以降と日本語フォントが必要です。Debian 13では次のパッケージ構成で確認しています。

```bash
sudo apt-get update
sudo apt-get install -y fontconfig fonts-noto-cjk \
  qt6-base-dev qt6-declarative-dev qt6-multimedia-dev \
  qml6-module-qtquick-controls qml6-module-qtmultimedia
```

ZIPを展開して必要に応じて実行権限を付けます。

```bash
chmod +x utautts tools/* runtime/utautts-openjtalk-features runtime/utautts-worldline-bridge
./utautts
```

Qtやデスクトップ環境が異なるディストリビューションでは同等のQt Quick・Qt Multimediaパッケージを導入してください。日本語が四角い記号になる場合は[トラブルシューティング](troubleshooting.md)を確認してください。

## ボイスバンクを追加する

実行ファイルと同じ階層の`voice`ディレクトリへ音源ごとにフォルダを分けて配置します。音源フォルダには使用する発音を定義した`oto.ini`が必要です。

配置したらGUIの「ファイル」→「音源を再読込」を選びます。「音源フォルダを開く」から配置先を直接開くこともできます。

GUI版には「足立レイ UTAU音源 ver3.5.0」を初期音源として同梱しています。利用前に[ボイスバンクの利用条件](voicebank.md)と音源内の文書を確認してください。追加した音源にはそれぞれの配布元が定める利用条件が適用されます。

## Server版

Server版にGUIは入っていません。ZIPを展開してWindowsでは次のように起動します。

```powershell
.\utautts-server.exe --voice-dir ".\voice"
```

Linuxでは次のとおりです。

```bash
chmod +x utautts-server runtime/utautts-openjtalk-features runtime/utautts-worldline-bridge
./utautts-server --voice-dir ./voice
```

起動すると`http://127.0.0.1:8080/`でコンソールUIを開けます。外部へ公開する前に[UtauTTS Server](server.md)の認証と入力制限を確認してください。
