# UtauTTS ドキュメント

## 利用者向け

- [GUIの使い方](gui.md): 文章入力、再生、保存、ショートカット、ドラッグ＆ドロップ
- [コマンドライン（CLI）](cli.md): CLIでの合成、オプション一覧
- [イントネーションとモーラ長の編集](manual-pitch.md): GUI・CLIでの韻律編集
- [手動調整から抑揚モデルを作る](prosody-model-training.md): 教師データ収集、監査、学習、評価、追加
- [辞書設定](dictionary.md): 表記ごとの読みの登録と適用ルール
- [トラブルシューティング](troubleshooting.md): runtime、Open JTalk、Rendererの問題
- [ボイスバンクの利用条件](voicebank.md): 同梱音源の出所、ライセンス、ハッシュ
- [UtauTTS Server](server.md): HTTPサーバーの起動方法、API、入力制限

## 開発者向け

- [リリーステスト](release-testing.md): 配布物の自動スモークテストと手動確認項目
- [構成](architecture.md): 合成パイプラインと標準構成
- [モデル／Rendererプラグイン](plugins.md): 検出、互換性、追加方法

標準の利用方法はrootの [README](../README.md) を参照してください。`docs/` の内容はリリーススクリプトによって配布物へコピーされます。研究途中の比較や実験記録は公開用ドキュメントには含めません。
