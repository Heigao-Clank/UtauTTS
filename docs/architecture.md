# 構成

UtauTTSはボイスバンク自体を学習せず、既存原音の選択・時間配置・接続方法を改善します。

1. `frontend`: 通常はKagome辞書で読みを作る。外部言語特徴を必要とするモデルでは、同梱OpenJTalk frontendが読み、アクセント句、核、単語境界、品詞を生成する。いずれもモーラとポーズへ分割する
2. `voicebank`: `oto.ini`を読み、各モーラの候補ラティスを作成。VCV優先度・原音設定の整合性をtarget score、境界音響差をjoin scoreとして、フレーズ全体の最良経路を動的計画法で選択
3. `prosody`: 固定規則または学習モデルでモーラ長を決定
4. `render`: 合成計画に従って波形を生成

境界フレーム抽出と手設計join scoreは`connection`へ集約し、`voicebank`の経路探索と`dataset`の学習例生成で共有します。これにより、学習時と推論時の特徴量定義がずれないようにしています。

`connection-train`は音源・groupを跨がない分割を行い、話者非依存の境界差分から小規模なロジスティック回帰を学習します。診断用として1隠れ層MLPも同じ特徴と推論経路で比較できます。`--join-model`を指定すると、予測確率のlogitを−2から＋2へ制限して−8から＋8の音響join scoreへ変換し、同一録音の順方向継続ボーナスと合わせてViterbi探索へ使用します。モデルを省略した場合は手設計join scoreへ戻ります。

Windows GUIはGoからWin32 APIを直接呼び、標準の`EDIT`・`BUTTON`・`COMBOBOX`だけを使用します。合成は別ゴルーチンで実行し、既存の`tts`パッケージを直接呼びます。

OpenJTalk frontendは`runtime/utautts-openjtalk-features.exe`として分離し、標準入力で文章JSON、標準出力で読み・モーラ列・疎な言語特徴JSONを交換します。`tts`はモデルmetadataを見て必要な場合だけhelperを起動し、返されたモーラ列をGo frontendの結果と一要素ずつ照合します。評価時に`ProsodyFeatures`を明示した場合は、helperを呼ばず固定特徴を使用します。

合成計画v9には原音、候補数、選択方式、join cost方式、target score、join score、学習モデルの接続確率、累積path score、実効タイミング、原音F0、目標F0、適用したイントネーション係数を記録します。旧`selection_score`は互換性のため残し、target scoreと選択された直前ユニットからのjoin scoreの和を格納します。実験的な`boundary_bridge_ms`を指定した場合は、通常接続を含む補修候補の判断を`boundary_repair_decisions`、適用した補修だけを`boundary_bridges`へ記録します。

原音選択は次の3方式を同じ候補集合で比較できます。

- `viterbi`: targetとjoinの合計をフレーズ全体で最適化。標準
- `greedy`: 直前に選択した原音とのjoinだけを見て逐次選択
- `target-only`: joinを使わず各位置のtargetだけで独立選択

aliasのtarget優先度は配列位置ではなく意味的tierで付けます。VCVまたは語頭用alias、CV、ワイルドカードの順で1段階ずつ下げ、ひらがな・カタカナという表記差には同じ点を与えます。prefix.mapの指定音階aliasを優先し、無印aliasは1段階下のフォールバックです。

促音に対応する原音がない場合だけ`<closure>`という無音閉鎖を挿入します。これにより音源のalias不足で発話全体を失敗させず、次の子音まで促音長を確保します。通常の促音原音が存在する場合はそちらを使用します。

手設計join scoreは、同じ録音内の自然な順序を優遇し、接続前後30msの正規化スペクトル、RMS、F0、有声・無声の一致を比較する決定的なベースラインです。学習特徴v2はこれらの差分にピッチ同期波形相関を加えます。モデル形式v3ではロジスティック回帰と1隠れ層MLPを保存でき、モデルv1/v2も読み込めます。WAVを解析できない場合は局所スコアだけで探索を継続します。探索量を制限するため各位置はtarget score上位32候補へ事前選別します。

`evaluation`は合成WAVのユニット境界について、クリック、音量差、スペクトル差、F0差を測定します。詳細は[接続品質の評価](evaluation.md)、学習例の形式は[接続学習データセット](connection-dataset.md)を参照してください。

## レンダラ

### waveform

原音をWSOLAで時間伸縮し、絶対時刻へ配置します。隣接ユニットは相補的にクロスフェードし、三重以上の重なりだけを正規化します。長すぎるVCVの発声先行は、母音末尾を残す範囲で圧縮します。

外部依存がなく、同じ入力から同じWAVを生成する標準レンダラです。

PitchFactorを与えない、または`ApplyPitch`を有効にしない`waveform`を発音明瞭度の固定基準とします。標準CLIでは`--apply-pitch`を省略するため、`--prosody`を指定してもF0のリサンプルは行いません。`--apply-pitch`、pitch-only、pitch contourは実験用です。韻律レンダラの評価では、この基準より文章の聞き取りやすさを落とさないことを最初の採用条件にします。Overlapが0、またはPreutteranceと同値でも瞬時切替にならないよう、最低6msの相補クロスフェードを使います。

### waveform-long（実験的）

同じWAV・oto.iniで行とoffsetが順方向に連続する原音を一つの長い区間としてWSOLA処理し、内部のクロスフェードを省きます。条件外は通常waveformへ戻ります。録音途中の別音素を誤って含めないよう条件を厳しくしていますが、大きな時間圧縮で客観指標が悪化する場合があるため、標準レンダラにはしていません。

### worldline（実験的）

OpenUtau 0.1.565由来のnative libraryを旧`PhraseSynth` C API経由で呼び、フレーズ単位のWORLD合成を行います。現行OpenUtauの`PhraseSynthV2`とは解析・時間写像・特徴混合の実装が異なるため、OpenUtauと同等のピッチ品質を再現した経路ではありません。次のレンダラ実験では現行`PhraseSynthV2`相当を別backendとして実装し、PitchFactor 1の明瞭度から比較します。

### worldline-v2（現行OpenUtau追試用）

現行OpenUtauの`PhraseSynthV2`から、44.1 kHz・10ms hop・2048 FFTのWORLDLINE-R v1特徴経路を依存なしで移植した実験backendです。各原音のF0・スペクトル包絡・非周期成分を解析し、子音と母音を別々に時間写像し、重複する特徴を混合してからフレーズ全体を一度だけ合成します。有声フレームだけへ密な絶対F0曲線を適用します。

同梱中のOpenUtau 0.1.565 `worldline.dll`には必要な解析APIがないため、このbackendには現行OpenUtauソースからビルドしたnative libraryを`--worldline`で明示します。標準配布backendにはまだ含めません。

足立レイ12文の聴感比較では、標準`waveform` 11、`worldline-v2` 0、同程度1となり、現行OpenUtau相当の特徴経路でも子音の歯抜けを解消できませんでした。棒読みv2対学習F0 v2も12件すべて同程度で、韻律差より子音欠落が支配しました。このbackendは実装差の調査用に限定し、標準候補にはしません。

### utau-classic（UTAU resampler追試用）

UTAU互換`resampler.exe`へ、原音F0に近いMIDI noteと5 tick間隔のcent曲線を渡し、加工済みの各原音を標準waveformと同じ時間配置・envelopeで接続する実験backendです。`--utau-resampler`を省略したWindows環境では`Program Files (x86)/UTAU/resampler.exe`を探索します。WORLD特徴化を介さずにUTAU/OpenUtau Classic型のピッチ加工を評価するための経路で、標準backendにはまだしません。

OpenUtau由来のWORLDフレーズ合成を別プロセスの.NETブリッジから呼び出します。各原音をF0・スペクトル包絡・非周期成分へ分解し、フレーズ単位で合成します。利用可能な`.frq`は自動的に渡します。

### worldline-hybrid（実験的）

WORLD合成を土台にします。各`oto.ini`固定範囲を30ms窓で調べ、周期を検出できない区間、または波形版より28%以上減衰した区間だけを原波形から復元します。切り替えは4msで行い、母音持続部の波形伸縮ノイズを戻さないようにします。

### worldline-hybrid-cv（実験的）

`worldline-hybrid`に加え、CV aliasの`oto.ini`固定範囲を時間・ピッチ調整済み原波形から必ず復元します。WORLDの無周期判定が特殊な合成音源の子音を見逃す場合に、子音の歯抜け感を減らせるか比較するための方式です。VCV原音と母音持続部には適用しません。

### worldline-hybrid-cv-balanced（実験的）

`worldline-hybrid-cv`で改善した子音の明瞭さを残しつつ、直接波形による粗い声質を抑える中間方式です。CV aliasの発音開始前からノート境界直後8msまでを最大85%復元し、それ以降の母音側は通常の`worldline-hybrid`と同じ検出方式を使います。

足立レイと固定12文を用いた初期評価では、全固定部復元版と従来hybrid版より明瞭でした。しかし、学習ピッチを実際に反映した後の複数音源評価では、WORLD共通の子音脱落と、原波形復元部のクリック・粗さが確認されました。この方式はWORLD系同士の比較候補として残しますが、標準レンダラにはしません。

### worldline-hybrid-cv-gentle（WORLD系の暫定標準候補）

`worldline-hybrid-cv-balanced`と同じCV発音開始区間を使い、直接波形の復元率を最大55%に抑えた中間方式です。balancedの発音明瞭度と、通常hybridの粗さの少なさの間を狙います。2026-08-11の聴感比較ではbalanced・gentle間の差は小さかったものの、通常hybridで目立った歯抜け感を避けながら調整できる候補として、今後のWORLD系比較の基準にします。

## 品質条件

- 不足原音や不正設定を黙って無視しない
- 同じ入力と設定から同じ結果を生成する
- 波形版を残し、レンダラ間を同一条件で比較できる
- 発音明瞭度、接続ノイズ、声質、韻律を別々に評価する
- ピッチ加工を伴う方式は、PitchFactorなしの`waveform`より聞き取りやすさを落とさない場合だけ採用する
