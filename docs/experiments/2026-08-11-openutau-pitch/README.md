# OpenUtauピッチ処理の追試

## 境界補修の終了判断

置換型の位相同期クロスフェードを標準`waveform+handcrafted`と比較した。結果は足立レイが標準5・拡張0・同程度6、utaが標準3・拡張0・同程度8、重音テトが標準2・拡張0・同程度10だった。差が分かる回答はすべて標準を選び、拡張側だけにクリックがある例もあったため棄却した。接続はPitchFactorなしの標準`waveform`と、任意の保守的母音末尾補修で一旦完成とする。

## OpenUtauとの実装差

調査対象はOpenUtau `29e0e16d1623cda79ba7c3724614d6129ba3b9d5`（2026-08-01）である。

OpenUtauのClassic rendererは、フレーズのピッチ曲線を5 tick間隔で各phoneへ補間し、phoneの基準MIDI noteからのcent差としてresamplerへ渡す。worldline resamplerはF0・スペクトル包絡・非周期成分を時間写像し、有声フレームへ目標F0を入れてから合成する。UtauTTSの`waveform --apply-pitch`のように、波形全体を線形リサンプルしてWSOLAで長さを戻す方式ではない。

現行OpenUtauのWORLDLINE-Rは、44.1 kHz、10 ms hopまたは512 sample hopの`PhraseSynthV2`を使う。各phoneのWORLD特徴を解析・時間写像し、重複区間のスペクトル包絡と非周期成分を混合した後、フレーズ全体の滑らかな絶対F0を有声フレームだけへ設定し、最後に一度だけ合成する。

従来のUtauTTS `worldline`は、同梱したOpenUtau 0.1.565のnative DLLを旧`PhraseSynth` C APIで呼ぶ。このDLLには現行`PhraseSynthV2`が使う`InitAnalysisConfig`、`WorldAnalysisF0In`、`WorldSynthesis`の公開APIがなく、現行OpenUtauと等価な比較ではなかった。

## worldline-v2最小移植

現行OpenUtauのnative libraryをソースからビルドし、WORLDLINE-R v1相当の特徴経路を依存なしでブリッジへ移植した。44.1 kHz・10 ms hop・2048 FFTに固定し、FRQを使ったF0、原音切り出し、子音と母音の別時間写像、SP/AP混合、voicing mask、フレーズ単位のWORLD合成を行う。

足立レイ「こんにちは。」をPitchFactor 1で生成した4境界のmean clickは、旧`worldline` 0.06216、標準`waveform` 0.03692、`worldline-v2` 0.01906だった。これは明瞭度や声質の合格を意味しないが、旧経路より悪化していない初期値である。

聴感用に2組を生成した。

- `out/listening/worldline-v2-flat-vs-waveform/public/index.html`: waveform棒読み対v2棒読み
- `out/listening/worldline-v2-flat-vs-v6/public/index.html`: v2棒読み対v2学習F0

各12組のWAVはすべて異なることをSHA-256で確認した。前者で新レンダラ自体の子音・声質、後者で同じレンダラにF0を与えた時のケロケロ・ゴロゴロ・ガラガラ感を分離評価する。

## 聴感結果と棄却

`listening-results10.json`の棒読み比較は、標準`waveform` 11、`worldline-v2` 0、同程度1だった。ほぼ全文でv2の歯抜け感が大幅に増えた。`listening-results11.json`のv2棒読み対v2学習F0は12件すべて同程度であり、イントネーション差よりv2共通の子音欠落が支配した。

原音切り出しを監査すると、v2は`oto.ini`のoffsetより余分に前を削ってはいなかった。WORLD解析にはoffsetの約20ms前から渡し、時間写像だけをoffsetから開始している。文頭では負のpreutteranceを配置できず、その分をskipするため最初の子音が短くなる問題はある。しかし文中の各子音区間ではv2のRMSが標準waveformより低いわけではなく、全体の歯抜けを前方切りすぎだけでは説明できない。

主因は、短い破裂・摩擦・無周期成分を10ms単位のWORLD特徴へ変換した際に、エネルギーを残したまま子音の識別に必要な時間構造が平均化されることだと判断する。これは以前の純WORLD試験と同じ失敗である。`worldline-v2`はOpenUtau実装差の切り分けには役立ったが、標準候補および学習F0の土台として棄却する。

UTAU本体は`resampler.exe`へ5 tick間隔のcent曲線を渡し、加工済みの各原音を`wavtool`で接続する。次はローカルにインストール済みのUTAU `resampler.exe`を使うClassic経路を比較し、PitchFactor 1の子音と声質を先に確認する。

## utau-classic初期実装

`utau-classic` backendを追加した。選択済み原音とFRQを一時領域へコピーし、原音F0に最も近いMIDI noteと5 tick間隔のcent曲線をUTAU `resampler.exe`へ渡す。加工済み原音は標準waveformと同じ発音位置、preutterance、overlap、release envelopeで接続する。標準backendは変更していない。

足立レイ「こんにちは。」の4境界ではmean click 0.01189、mean peak click 0.10214、mean F0差10.61 centsだった。旧`worldline`のmean click 0.06216、`worldline-v2`の0.01906、標準`waveform`の0.03692より低い初期値である。

次の聴感比較を生成した。

- `out/listening/utau-classic-flat-vs-waveform/public/index.html`: waveform棒読み対UTAU resampler棒読み
- `out/listening/utau-classic-flat-vs-v6/public/index.html`: UTAU resampler棒読み対UTAU resampler学習F0

両方とも12試行・24 WAVが揃い、全12ペアが異なることをSHA-256で確認した。外部resamplerの起動回数を抑えるため、同一原音・同一パラメータの結果はプロセス内でキャッシュする。

## 次の判断

1. UTAU `resampler.exe`互換のClassic実験経路を作り、まずPitchFactor 1で標準`waveform`と比較する。
2. Classic経路が子音と声質を保てる場合だけ、±25・±50・±100 centsの単一山型F0で安全な加工幅を求める。
3. 小幅曲線で粗さが増えない場合、アクセント句単位の基準F0と連続な残差を予測し、5 tick間隔のcent曲線としてresamplerへ与える。
4. 同一音源群に多音階候補がある場合だけ、予測F0を候補選択costにも使う。

この順序により、イントネーションモデルの誤差と、F0を音へ変換するレンダラの歪みを分離する。

## 実OpenUtauプロジェクトの同定

ローカルの`.ustx`からBPM、expression既定値、各trackのsinger・phonemizer・renderer・resampler・wavtoolを調べる`cmd/openutau-audit`を追加した。個別プロジェクトのファイル名・パス・hashは研究文書へ保存しない。

監査した足立レイのプロジェクトは次の設定だった。

- singer: `足立レイver3.1.2`
- renderer: `CLASSIC`
- resampler: `worldline`
- wavtool: `convergence`
- velocity: 100
- volume: 100
- modulation: 0
- BPM: 120

監査した重音テトのプロジェクトは`WORLDLINE-R`だった。したがって、「OpenUtauでは自然だった」という観察には少なくとも二経路が混在しており、従来のUTAU標準`resampler.exe`試験は足立レイの実設定を再現していなかった。

OpenUtauソースを確認すると、この`worldline`は外部exeではなく、native libraryの`Resample`をphoneごとに呼ぶ内蔵resamplerである。`convergence`も外部wavtoolではなく、各phoneへenvelopeを掛け、重複する周期区間の位相を補正してから加算する内蔵`SharpWavtool`である。phrase全体を一度WORLD合成する`WORLDLINE-R`/`worldline-v2`とは別方式である。

## 決定的ピッチスイープ

`render.PitchCurve`を追加し、発話先頭からの`frame_ms`と連続cent列をWORLD系およびClassic系rendererへ渡せるようにした。cent値はlog-F0空間で補間する。既知の劣化方式へ暗黙に戻らないよう、標準`waveform`と`waveform-long`はフレーム曲線を明示的に拒否する。

`cmd/renderer-sweep`は標準waveform基準と次の9ケースを同じ選択原音から生成し、原音、目標F0、renderer設定、実行ファイルhash、OpenUtauプロジェクト監査を`manifest.json`へ残す。

- flat
- ±25 cents
- ±50 cents
- ±100 cents
- 5 tick間隔の単一山型（最大+100 cents）
- 1フレーム幅で0から+100 centsへ遷移する急峻な段差

足立レイ「あいうえお」の生成物は`out/renderer-sweep/adachi-openutau-classic-worldline`にある。

## CLASSIC + worldline + convergence経路

`openutau-classic-worldline` backendを追加した。OpenUtauの構成に合わせ、各原音をnative `Resample`へ個別に渡し、5 tick相当の目標pitch bendで加工してから、重複区間を位相整列しenvelope付きで接続する。現時点の位相整列は依存を増やさない正規化相関方式であり、OpenUtau `SharpWavtool`のNWaves帯域フィルタとはbit exactではない。この差はmanifestのwarningへ残す。

現行OpenUtauからビルドしたworldline DLLを使い、足立レイのwaveform基準1本と9本の決定曲線を最後まで生成できた。flat品質のブラインド比較は次に生成済みである。

- `out/listening/openutau-classic-worldline-flat-vs-waveform/public/index.html`

12試行で、標準`waveform`対`openutau-classic-worldline`のrenderer差だけを評価する。この結果が原音保持・子音明瞭度で不合格なら、±25〜100 centsの聴感採用判定や新しい韻律モデル学習へ進まない。

## CLASSIC + worldline移植のflat聴感結果

`listening-results12.json`を集計した結果は次のとおりだった。

- 標準`waveform`: 12
- `openutau-classic-worldline`: 0
- 同率: 0

全12文で標準`waveform`が選ばれたため、現在の`openutau-classic-worldline`はflat品質の採用条件を明確に満たさない。したがって、この実装を使った±25〜100 cents、山型、段差、既存v5/v6、新しい韻律モデルの聴感比較には進まない。

ただし、この結果だけでOpenUtauの`CLASSIC + worldline + convergence`自体を棄却してはいけない。現在の移植には少なくとも次の非同一部分が残る。

- `SharpWavtool`のNWaves IIR peak filterではなく、正規化相関で位相整列している。
- OpenUtauの5点phoneme envelopeを完全には移植していない。
- `RenderPhone`由来のleading、tail intrusion、tail overlap、duration correctionをUtauTTSのplan timingから近似している。
- OpenUtau実書き出しと同一入力で波形差を測っていない。

よって今回の判定は「OpenUtau方式の棄却」ではなく「現在の近似移植の棄却」である。次は自然だと確認できるOpenUtau実書き出しを基準にし、同一原音・同一oto・同一ノート・同一pitch・同一expressionで、原音単体出力とconvergence後出力を段階比較する。OpenUtau実出力を得られない間は、近似移植を調整して聴感比較を繰り返さない。

## OpenUtau timingと5点envelopeの忠実化

その後の診断で、母音だけをraw波形からWORLD波形へ切り替える方式は、短い加工区間がモーラごとに反復することで高速の音色震えを作ると判明した。全文一括WORLDや周波数分割は震えを避けられたが、歯抜け、EQ感、非調和な筒鳴りとの交換になったため棄却した。

per-phone Classic経路へ戻り、OpenUtauと同じpreutterance/overlap検証、tail intrusion/overlap、duration correction、負値を許すskipOver、50ms単位のrequired duration、共通phrase原点、resampler出力全長へ掛ける5点linear envelopeを`openutau-classic-worldline-faithful`へ移植した。

歯抜けに敏感な4文で従来近似版と比較した結果は、faithful版3、従来版1だった。聴感でも不自然な箇所が一連の実験で最少クラスと評価された。さらにSharpWavtoolのQ=5狭帯域位相収束を分離移植したが、比較結果は従来の正規化相関版3、OpenUtau式0、同程度1だった。実装上の忠実さより実音を優先し、位相収束だけは正規化相関を維持する。

## 最終韻律比較

固定したfaithful renderer上で、frame TCN v8、`pyopenjtalk.tts`実音声から抽出したF0、輪郭なしを比較した。OpenJTalk F0との比較はv8が6、OpenJTalkが0、同程度6だった。続くv8対輪郭なしはv8が10、輪郭なしが0、同程度2で、挨拶、母音連続、無声子音、拗音、長母音、促音、鼻音、摩擦音、文末、疑問文の全非同率回答でv8が選ばれた。

現時点の採用構成は次である。

- renderer: `openutau-classic-worldline-faithful`
- prosody model: frame TCN v8
- prosody mode: pitch-only
- convergence: 正規化相関
- timing/envelope: OpenUtau準拠

以後の再学習やrenderer変更は、この構成に12文の非同率回答で負けず、歯抜け・震え・筒鳴りを再発させないことを必須gateとする。
