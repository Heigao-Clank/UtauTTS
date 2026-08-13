using Microsoft.ML.OnnxRuntime;
using Microsoft.ML.OnnxRuntime.Tensors;

namespace UtauTTS.WorldlineBridge;

// OpenUtau WORLDLINE-R2 keeps WORLD analysis on the CPU, converts those
// features to a mel spectrogram with its small fixed model, and runs the
// PC-NSF-HiFiGAN vocoder through ONNX Runtime. DirectML is the GPU path used
// by OpenUtau on Windows; the separate CPU engine exists for diagnostics.
internal static class WorldlineR2 {
    private const int HopSize = 512;
    private const int LeftPadding = 4;

    public static void Render(IntPtr library, Manifest manifest, bool directML) {
        ValidateModel(manifest.MelModelPath, "OpenUtau mel model");
        ValidateModel(manifest.VocoderModelPath, "PC-NSF-HiFiGAN vocoder model");

        var features = WorldlineV2.BuildFeatures(library, manifest, HopSize);
        var paddedFrames = (int)Math.Ceiling((features.TotalFrames + 12) / 16.0) * 16;
        var spectrumSize = features.FftSize / 2 + 1;
        var f0 = new float[paddedFrames];
        var spectrum = new float[paddedFrames * spectrumSize];
        var aperiodicity = Enumerable.Repeat(1f, paddedFrames * spectrumSize).ToArray();
        for (var frame = 0; frame < features.TotalFrames; frame++) {
            var destinationFrame = frame + LeftPadding;
            f0[destinationFrame] = (float)features.F0[frame];
            var sourceOffset = frame * spectrumSize;
            var destinationOffset = destinationFrame * spectrumSize;
            for (var bin = 0; bin < spectrumSize; bin++) {
                spectrum[destinationOffset + bin] = (float)features.Spectrum[sourceOffset + bin];
                aperiodicity[destinationOffset + bin] = (float)features.Aperiodicity[sourceOffset + bin];
            }
        }

        var f0Tensor = new DenseTensor<float>(f0, [1, paddedFrames]);
        var spectrumTensor = new DenseTensor<float>(spectrum, [1, paddedFrames, spectrumSize]);
        var aperiodicityTensor = new DenseTensor<float>(aperiodicity, [1, paddedFrames, spectrumSize]);
        using var melSession = new InferenceSession(Path.GetFullPath(manifest.MelModelPath));
        var melInputs = new List<NamedOnnxValue> {
            NamedOnnxValue.CreateFromTensor("f0", f0Tensor),
            NamedOnnxValue.CreateFromTensor("sp_env", spectrumTensor),
            NamedOnnxValue.CreateFromTensor("ap", aperiodicityTensor),
        };
        using var melResults = melSession.Run(melInputs);
        var mel = melResults.First(result => result.Name == "mel").AsTensor<float>().ToArray();
        var melDimensions = melResults.First(result => result.Name == "mel").AsTensor<float>().Dimensions.ToArray();

        using var options = directML ? CreateDirectMLOptions(manifest.OnnxDeviceId) : new SessionOptions();
        try {
            using var vocoderSession = new InferenceSession(Path.GetFullPath(manifest.VocoderModelPath), options);
            var melTensor = new DenseTensor<float>(mel, melDimensions);
            var vocoderInputs = new List<NamedOnnxValue> {
                NamedOnnxValue.CreateFromTensor("mel", melTensor),
                NamedOnnxValue.CreateFromTensor("f0", f0Tensor),
            };
            using var results = vocoderSession.Run(vocoderInputs);
            var raw = results.First().AsTensor<float>().ToArray();
            var start = LeftPadding * HopSize;
            var count = Math.Min(features.TotalFrames * HopSize, Math.Max(0, raw.Length - start));
            var output = raw.AsSpan(start, count).ToArray();
            EaseEdges(output, HopSize);
            Program.WritePCM16(manifest.OutputPath, features.SampleRate, output);
        } catch (OnnxRuntimeException error) when (directML) {
            throw new InvalidOperationException(
                $"DirectML WORLDLINE-R2 failed on GPU device {manifest.OnnxDeviceId}; " +
                "check the device ID, graphics driver, and model compatibility", error);
        }
    }

    private static SessionOptions CreateDirectMLOptions(int deviceId) {
        var options = new SessionOptions {
            EnableMemoryPattern = false,
            ExecutionMode = ExecutionMode.ORT_SEQUENTIAL,
        };
        options.AppendExecutionProvider_DML(deviceId);
        return options;
    }

    private static void ValidateModel(string path, string description) {
        if (string.IsNullOrWhiteSpace(path)) {
            throw new InvalidDataException($"{description} path is not configured");
        }
        if (!File.Exists(path)) {
            throw new FileNotFoundException($"{description} was not found", path);
        }
    }

    private static void EaseEdges(float[] samples, int length) {
        var count = Math.Min(samples.Length, length);
        for (var index = 0; index < count; index++) {
            samples[index] *= (float)index / count;
            samples[^(index + 1)] *= (float)(count - index) / count;
        }
    }
}
