# 手動調整から抑揚モデルを作る

UtauTTSのGUIでv8の自動イントネーションを調整し、その操作を教師データに小さな補正モデルを学習できます。生成されるversion 11モデルは固定した`frame-intonation-v8`とモーラ単位の補正headを一つのJSONへ収めた自己完結型モデルです。音声やボイスバンクそのものを学習するわけではありません。

この機能は実験的です。少量の教師データから生成できることとv8より自然になることは同じではありません。必ず学習に使っていない文章でv8と比較してください。

## 必要なもの

- UtauTTSのソースツリー、または`tools/`と`models/frame-intonation-v8.json`を含む開発用配布物
- Python 3.10以降
- Python packageのNumPyとPyTorch
- 教師データの試聴に使うボイスバンクとRenderer

PowerShellでUtauTTSのルートディレクトリを開いて必要なPython packageを準備します。既存のPython環境へ影響させたくないなら仮想環境を使ってください。

```powershell
python -m venv .venv
.\.venv\Scripts\Activate.ps1
python -m pip install --upgrade pip
python -m pip install numpy torch
```

学習はCPUだけで実行できます。現在の学習ツールはCPUを使うのでGPUは必要ありません。

## 1. 教師データを収集する

1. GUIの「設定」→「設定...」で「開発者モード」を有効にします。
2. 「設定」→「抑揚の教師データ生成」を開きます。
3. 文章セット、ボイスバンク、Renderer、音高、音源タイプを選んで「新しく開始」を押します。
4. 表示された文章を合成して聴き必要な点だけピッチグラフで調整します。
5. 結果を採用するなら「OK・次へ」を押します。変更しなかった文章も確認画面で明示的に採用すると0補正の教師になります。
6. 判断できない文章は「スキップ」を使用します。
7. 文章セットが終わったら「教師データを書き出す」を押します。

一度も合成していない文章は採用できません。収集中の状態は自動保存されて同じv8と辞書を利用できるなら「途中から再開」で続けられます。

文章セットには動作確認向けの「基本確認セット（10文）」とJSUT BASIC5000から抑揚や文長の分布を見て選んだ50文セットが6個あります。最初は10文で操作を確認してから、BASIC5000セットを一つずつ集めるのがいいと思います。一回のsession中は文章セットが固定されます。すべてを一度に終わらせる必要はなく書き出した複数のJSONLを学習時にまとめられます。

BASIC5000文章はJSUTの音声ではなくテキストだけを使っています。元テキストには田中コーパス（CC BY 2.0）、Wikipedia（CC BY-SA 3.0）、JSUT独自文（CC BY-SA 4.0）が含まれます。出典と条件は[`licenses/JSUT-DATA-AND-LABELS.txt`](../licenses/JSUT-DATA-AND-LABELS.txt)に記載しています。教師JSONLを再配布する場合もこの出典情報を一緒に示してください。

書き出しでは次の2ファイルが生成されます。

- `*.jsonl`: 採用した文章、読み、v8予測値、手動補正、編集mask、言語特徴、合成条件
- `*-report.json`: session、採用数、skip数、使用したv8などの概要

`.utautts`プロジェクトは教師データとして取り込みません。学習対象になるのは専用画面で確認して採用したJSONLだけです。

## 2. データを監査する

学習前に配列長、v8のidentity、非数値、pauseの誤編集などを検査します。

```powershell
python tools/audit-manual-intonation-data.py `
  .\data\manual-prosody-session1.jsonl `
  .\data\manual-prosody-session2.jsonl `
  --out .\out\prosody\manual-prosody-audit.json
```

エラーがあれば終了コード2になって学習へ進みません。reportでは特に次を確認してください。

- `records`: 採用した発話数
- `morae.edited_ratio`: 手動で変更した点の割合
- `offset_cents`: 補正方向と大きさ
- `model_hashes`: 収集に使ったv8が統一されているか
- `prompt_packs`: 各文章セットから採用した発話数
- `warnings`: 少量データ、補正方向の偏り、±120 cent超など

JSONLには入力文章、読み、ボイスバンクIDなどが入ります。共有や公開の前に内容と各音源の利用条件を確認してください。

## 3. モデルを学習する

次の例では複数sessionをまとめて3つのseedからvalidation成績が最もよいモデルを選びます。

```powershell
python tools/train-manual-intonation-residual.py `
  --dataset .\data\manual-prosody-session1.jsonl .\data\manual-prosody-session2.jsonl `
  --base-model .\models\frame-intonation-v8.json `
  --out .\out\prosody\my-manual-prosody-v1.json `
  --report .\out\prosody\my-manual-prosody-v1-training-report.json `
  --model-id my-manual-prosody-v1 `
  --display-name "My manual prosody v1" `
  --description "自分で確認した抑揚調整から学習" `
  --epochs 240 `
  --hidden 16 `
  --seeds 23,29,41
```

`--model-id`はGUI、CLI、Serverでモデルを指定する一意なIDです。英小文字、数字、ハイフンを使った後から変えなくて済む名前を推奨します。同じIDのモデルを`models/`へ複数置くと起動時に重複エラーになります。

複数JSONLに同じ文章と読みがある場合は同じgroupとして扱って引数で後に指定したsessionの採用結果を使います。異なるv8、frontend、辞書fingerprintのデータは混在させられません。

文章groupのhashを使っておおむねtrain 80%、validation 10%、test 10%へ固定分割します。少量データでどれかが空になる場合だけ最低1発話を決定的に割り当てます。同じ文章の再編集が別splitへ分かれることはありません。

学習targetは手動補正を±120 centへ制限した値です。編集点を強い教師、採用済み未編集点を弱い0補正教師としてdilation 1、2、4の小型TCNを学習します。生成JSONにはv8のframe headも入るので実行時に別のv8 JSONを参照しません。

## 4. 学習reportを判断する

reportの`metrics`には各splitについて`zero`と`model`があります。

- `zero`: v8をそのまま使い、補正を一切予測しないbaseline
- `model`: 学習した補正head
- `edited_mae_cents`: 実際に編集した点だけの平均絶対誤差
- `weighted_mae_cents`: 編集点と確認済み未編集点を教師weight込みで評価した誤差
- `portable_max_abs_error_cents`: 学習時推論とexport形式の推論差

まずvalidationの`weighted_mae_cents`がzero baselineを下回るか確認します。編集点だけ改善して未編集点を必要もなく動かすモデルではポン出し品質が悪化するかもしれません。testは学習設定を選び終えたあとの最終確認にだけ使います。

データ量の目安は次のとおりです。

- 約10発話: schema、収集操作、学習経路の動作確認
- 約30発話: 補正分布と単純baselineの確認
- 約100発話: 個人補正モデルの初回評価
- 200～300発話: 未学習文章の聴感比較と採用判断

発話数だけでなく疑問文、複数のアクセント句、長文、促音、長音、鼻音、異なるアクセント核位置を入れます。BASIC5000セットはこの偏りを減らすために選んでいますが疑問文が少ないので基本確認セットも併用してください。学習済み文章を再生して自然でも汎化性能の証明にはなりません。

## 文章セットを再生成する

同じJSUT BASIC5000 transcriptから標準セットを作り直す場合は`pyopenjtalk`を導入して次を実行します。選抜は決定的で50文ごとのセット間でも特徴が偏りにくいように配分されます。

```powershell
python -m pip install pyopenjtalk
python tools/select-jsut-prosody-prompts.py `
  --transcript .\data\jsut\basic5000\transcript_utf8.txt `
  --supplement .\tools\prosody-prompts-original-ja-v1.json `
  --out .\qt\prosody-prompts-ja-v1.json `
  --report .\out\prosody\jsut-basic5000-prosody-v1-selection-report.json `
  --count 300 `
  --pack-size 50
```

reportには入力transcriptのSHA-256、選抜条件、各セットのprompt IDと特徴coverageが残ります。

## 5. UtauTTSへ追加する

評価したモデルJSONを`models/`へコピーします。

```powershell
Copy-Item `
  -LiteralPath .\out\prosody\my-manual-prosody-v1.json `
  -Destination .\models\my-manual-prosody-v1.json
```

GUIを再起動すると「抑揚モデル」の一覧へ表示されます。モデル一覧は起動時に読み込まれるので実行中にコピーした場合は再起動が必要です。

CLIでは`models/`へコピーせず検索directoryを追加して試すこともできます。

```powershell
.\utautts-cli.exe `
  --model-dir .\out\prosody `
  --prosody my-manual-prosody-v1 `
  --prosody-pitch-only `
  --apply-pitch `
  --renderer openutau-worldline-r-faithful `
  --voicebank .\voice\my-voicebank `
  --text "こんにちは、今日はいい天気です。" `
  --out .\out\my-manual-prosody.wav
```

最初はv8と作成モデルを同じ文章、ボイスバンク、Rendererで比較してください。作成モデルのほうが悪ければJSONを`models/`から削除すれば元に戻せます。教師JSONLまで削除する必要はありません。

## 再学習時の注意

- 元のJSONLは編集せず、sessionごとに保存します。
- dataset、base model、引数、seed、reportを一緒に保管します。
- v8が更新された場合は古いv8で収集したデータをそのまま新しいv8の補正学習へ使いません。
- 同じ文章を再調整した場合は新しいsessionを`--dataset`の後ろへ指定します。
- モデルJSONには教師文章そのものは入りませんが、学習した特徴語彙や個人の調整傾向を含みます。公開範囲を自分で判断してください。
