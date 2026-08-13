# OpenUTAU WORLDLINE-R2 GPU renderer

`openutau-worldline-r2-directml`は、現行OpenUtauのWORLDLINE-R2と同じ構成をUtauTTSへ移した実験的Rendererです。WORLD解析とmel変換はCPU、PC-NSF-HiFiGAN vocoderはWindows DirectMLでGPU実行します。NVIDIA CUDA専用ではないため、DirectML対応のNVIDIA・AMD・Intel GPUを利用できます。

既存の`openutau-classic-worldline-faithful`とは合成方式も音も異なります。Classic faithfulのGPU版ではなく、独立したRendererとして選択してください。

## 必要なファイル

- `InitAnalysisConfig`と`WorldAnalysisF0In`をexportする現行OpenUtauの`worldline.dll`
- OpenUtauの固定`mel.onnx`。release buildは`runtime/worldline-r2-mel.onnx`へhash検証付きで取得します
- OpenVPI公式PC-NSF-HiFiGAN 44.1kHz / hop 512 / 128-binモデル

vocoder weightはCC BY-NC-SA 4.0で、商用利用できないためUtauTTSには同梱しません。条件を確認して導入する場合は次を実行できます。

```powershell
.\tools\install-worldline-r2-vocoder.ps1 `
  -OutputPath .\models\worldline-r2-vocoder.onnx `
  -AcceptNonCommercialLicense
```

2026-08-13時点のOpenUtau安定版0.1.565同梱DLLは必要な解析APIを持ちません。R2の検証には現行OpenUtau source commit `29e0e16d1623cda79ba7c3724614d6129ba3b9d5`からビルドした`worldline.dll`を指定してください。

## 実行

```powershell
go run ./cmd/utautts-cli `
  --voicebank "release/UtauTTS/voice/足立レイver3.5.0" `
  --text "こんにちは。" `
  --renderer openutau-worldline-r2-directml `
  --worldline "path/to/current/worldline.dll" `
  --worldline-r2-mel "path/to/worldline-r2-mel.onnx" `
  --worldline-r2-vocoder "models/worldline-r2-vocoder.onnx" `
  --onnx-device 0 `
  --out "r2-directml.wav"
```

比較用CPU版はRendererを`openutau-worldline-r2-cpu`へ変更します。DirectML版は失敗時にCPUへfallbackしません。誤ったdevice ID、非対応driver、モデル非互換は明示的なエラーになります。

## 現在の性能

検証機で足立レイを別processで生成した結果、6.47秒発話はCPU 8.76秒、DirectML 9.51秒で短文ではGPU初期化コストが勝ちました。19.36秒発話はCPU 24.54秒、DirectML 22.05秒となり、DirectMLが約10%高速でした。CPU/GPU出力のPSNRはそれぞれ159.37 dB、161.61 dBで、量子化後も実質同一です。短文性能の次の課題はbridge常駐化とvocoder sessionの再利用です。

参照実装とモデル配布元:

- https://github.com/openutau/OpenUtau/blob/master/OpenUtau.Core/Classic/WorldlineRenderer.cs
- https://github.com/openutau/OpenUtau/blob/master/OpenUtau.Core/Util/Onnx.cs
- https://github.com/openvpi/vocoders/releases/tag/pc-nsf-hifigan-44.1k-hop512-128bin-2025.02
