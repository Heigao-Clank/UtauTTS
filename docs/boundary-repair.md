# 境界補修実験

UtauTTS のボイスバンクは、専用 TTS コーパスに比べて録音候補が疎です。そのため、現在の接続候補だけでは音響的に悪い境界が残ることがあります。境界補修実験は、読みのモーラ列を変更せず、通常接続と位相同期した母音末尾候補を境界ごとに比較します。

この機能は既定では無効です。

```powershell
go run ./cmd/utautts-cli `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは" `
  --boundary-bridge-ms 20 `
  --boundary-bridge-threshold 0 `
  --out out/boundary-bridge.wav `
  --plan-out out/boundary-bridge-plan.json
```

`--boundary-bridge-ms` は補助候補の最大幅です。実装では最大値以下の8・12・20ms候補を作り、直前母音内を±5ms探索して波形相関が最大になる位置を選びます。さらにクロスフェード入口・中央・出口付近を比較し、境界区間全体のピーク差が3%以上改善し、微分RMSを悪化させない候補だけを採用します。通常接続は常に候補として残り、合計時間は増やしません。

`--boundary-bridge-threshold` 以下の handcrafted join score を持つ境界だけが対象です。通常の出力を維持するには`--boundary-bridge-ms 0`を使います。

複数方式を同じ入力で比較する場合は、次のコマンドで `handcrafted-viterbi-bridge` ケースを追加できます。

```powershell
go run ./cmd/connection-compare `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは" `
  --boundary-bridge-ms 20 `
  --out out/boundary-bridge-compare
```

合成計画v9の`boundary_repair_decisions`には、通常接続を維持した境界も含め、候補数、選択方式、幅、lag、波形相関、補修前後のピーク差と微分RMSが記録されます。実際に適用された補修だけは`boundary_bridges`にも残ります。クリック、RMS、スペクトル、F0 の値は `connection-eval` で確認できますが、最終判断は聞き取りやすさ、歯抜け感、クリック、ざらつきを分けた聴感比較で行います。

現在の実装は候補ラティスへ補助エッジを追加する前の決定論的な実験です。効果が確認できた場合に限り、通常接続・クロスフェード・母音継続・VCV/長いユニットを既存の Viterbi 選択肢として統合します。
