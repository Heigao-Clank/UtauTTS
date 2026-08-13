using System.Runtime.InteropServices;

namespace UtauTTS.WorldlineBridge;

// Minimal, dependency-free port of OpenUtau's PhraseSynthV2 feature path.
// It intentionally supports the WORLDLINE-R v1 condition first: 44.1 kHz,
// 441-sample hop, 2048-point FFT, and one WORLD synthesis per phrase.
internal static class WorldlineV2 {
    [StructLayout(LayoutKind.Sequential)]
    private struct AnalysisConfig {
        public int Fs;
        public int HopSize;
        public int FftSize;
        public float F0Floor;
        public double FrameMS;
    }

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate void InitAnalysisConfig(ref AnalysisConfig config, int fs, int hopSize, int fftSize);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate int EstimateF0(float[] samples, int length, int fs, double framePeriod, int method, out IntPtr f0);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate void AnalyzeF0In(ref AnalysisConfig config, float[] samples, int sampleCount,
        double[] f0, int frameCount, double[] spectrum, double[] aperiodicity);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate int Synthesize(double[] f0, int f0Length,
        double[] spectrum, [MarshalAs(UnmanagedType.I1)] bool isMgc, int spectrumSize,
        double[] aperiodicity, [MarshalAs(UnmanagedType.I1)] bool isBap, int fftSize,
        double framePeriod, int fs, out IntPtr output,
        double[] gender, double[] tension, double[] breathiness, double[] voicing);

    private sealed class Segment {
        public required double[] F0;
        public required double[] Spectrum;
        public required double[] Aperiodicity;
        public int SkipFrames;
        public int P0;
        public int P1;
        public int P3;
        public int P4;
    }

    internal sealed class Features {
        public required int SampleRate;
        public required int HopSize;
        public required int FftSize;
        public required int TotalFrames;
        public required double[] F0;
        public required double[] Spectrum;
        public required double[] Aperiodicity;
    }

    public static void Render(IntPtr library, Manifest manifest) {
        if (manifest.SampleRate != 44100) {
            throw new InvalidDataException($"worldline-v2 supports 44100 Hz only; got {manifest.SampleRate} Hz");
        }
        var synthesize = Load<Synthesize>(library, "WorldSynthesis");
        var features = BuildFeatures(library, manifest, 441);
        var totalFrames = features.TotalFrames;
        var config = new AnalysisConfig {
            Fs = features.SampleRate,
            HopSize = features.HopSize,
            FftSize = features.FftSize,
            FrameMS = features.HopSize * 1000.0 / features.SampleRate,
        };
        var spectrumSize = config.FftSize / 2 + 1;
        var gender = Enumerable.Repeat(0.5, totalFrames).ToArray();
        var tension = Enumerable.Repeat(0.5, totalFrames).ToArray();
        var breathiness = Enumerable.Repeat(0.5, totalFrames).ToArray();
        var voicing = Enumerable.Repeat(1.0, totalFrames).ToArray();
        var length = synthesize(features.F0, features.F0.Length, features.Spectrum, false, spectrumSize,
            features.Aperiodicity, false, config.FftSize, config.FrameMS, config.Fs, out var output,
            gender, tension, breathiness, voicing);
        if (length <= 0 || output == IntPtr.Zero) throw new InvalidOperationException("WorldSynthesis returned no audio");
        try {
            var values = new double[length];
            Marshal.Copy(output, values, 0, length);
            Program.WritePCM16(manifest.OutputPath, config.Fs, values.Select(value => (float)value).ToArray());
        } finally {
            Marshal.FreeCoTaskMem(output);
        }
    }

    internal static Features BuildFeatures(IntPtr library, Manifest manifest, int hopSize) {
        if (manifest.SampleRate != 44100) {
            throw new InvalidDataException($"WORLDLINE feature synthesis supports 44100 Hz only; got {manifest.SampleRate} Hz");
        }
        var init = Load<InitAnalysisConfig>(library, "InitAnalysisConfig");
        var estimate = Load<EstimateF0>(library, "F0");
        var analyze = Load<AnalyzeF0In>(library, "WorldAnalysisF0In");
        var config = new AnalysisConfig();
        init(ref config, 44100, hopSize, 2048);

        var segments = manifest.Units.Select(unit => BuildSegment(unit, config, estimate, analyze)).ToArray();
        var totalFrames = segments.Max(segment => segment.P4) + 1;
        var spectrumSize = config.FftSize / 2 + 1;
        var f0 = new double[totalFrames];
        var spectrum = Enumerable.Repeat(1e-12, totalFrames * spectrumSize).ToArray();
        var aperiodicity = Enumerable.Repeat(1.0, totalFrames * spectrumSize).ToArray();
        var dirty = new bool[totalFrames];

        foreach (var segment in segments) {
            for (var frame = segment.P0; frame < segment.P4 && frame < totalFrames; frame++) {
                var weight = 1.0;
                if (frame < segment.P1) {
                    weight = (double)(frame - segment.P0) / Math.Max(1, segment.P1 - segment.P0);
                } else if (frame >= segment.P3) {
                    weight = (double)(segment.P4 - frame) / Math.Max(1, segment.P4 - segment.P3);
                }
                var sourceFrame = segment.SkipFrames + frame - segment.P0;
                if (sourceFrame < 0 || sourceFrame >= segment.F0.Length) continue;
                if (!dirty[frame] || weight > 0.5) f0[frame] = segment.F0[sourceFrame];
                var outputOffset = frame * spectrumSize;
                var sourceOffset = sourceFrame * spectrumSize;
                for (var bin = 0; bin < spectrumSize; bin++) {
                    spectrum[outputOffset + bin] += segment.Spectrum[sourceOffset + bin] * weight;
                    var oldWeight = dirty[frame] ? 1.0 - weight : 0.0;
                    var newWeight = dirty[frame] ? weight : 1.0;
                    aperiodicity[outputOffset + bin] =
                        aperiodicity[outputOffset + bin] * oldWeight + segment.Aperiodicity[sourceOffset + bin] * newWeight;
                }
                dirty[frame] = true;
            }
        }

        for (var frame = 0; frame < totalFrames; frame++) {
            if (f0[frame] > config.F0Floor) {
                f0[frame] = manifest.F0Curve[Math.Min(frame, manifest.F0Curve.Length - 1)];
            }
        }
        return new Features {
            SampleRate = config.Fs,
            HopSize = config.HopSize,
            FftSize = config.FftSize,
            TotalFrames = totalFrames,
            F0 = f0,
            Spectrum = spectrum,
            Aperiodicity = aperiodicity,
        };
    }

    private static Segment BuildSegment(Unit unit, AnalysisConfig config, EstimateF0 estimate, AnalyzeF0In analyze) {
        var (sampleRate, source) = Program.ReadPCM16(unit.Source);
        if (sampleRate != config.Fs) throw new InvalidDataException($"{unit.Source} is {sampleRate} Hz; expected {config.Fs} Hz");
        var samples = source.Select(value => (float)value).ToArray();
		var sourcePeak = samples.Select(Math.Abs).DefaultIfEmpty(0).Max();
        var f0 = Estimate(samples, config, estimate);
		ApplyFRQ(unit.FrqPath, f0, config.HopSize, config.F0Floor);

        var sourceStartFrame = Math.Max(0, (int)(unit.OffsetMs / config.FrameMS));
        var sourceEndMS = unit.CutoffMs < 0
            ? unit.OffsetMs - unit.CutoffMs
            : samples.Length * 1000.0 / config.Fs - unit.CutoffMs;
        var sourceEndFrame = Math.Min(f0.Length, (int)Math.Ceiling(sourceEndMS / config.FrameMS));
        if (sourceEndFrame <= sourceStartFrame) throw new InvalidDataException($"cutoff before offset: {unit.Source}");

        var trimStartFrame = Math.Max(0, sourceStartFrame - 2);
        var trimEndFrame = Math.Min(f0.Length, sourceEndFrame + 2);
        sourceStartFrame -= trimStartFrame;
        sourceEndFrame -= trimStartFrame;
        f0 = f0[trimStartFrame..trimEndFrame];
        var trimStartSample = trimStartFrame * config.HopSize;
        var trimLength = Math.Min(samples.Length - trimStartSample, (trimEndFrame - trimStartFrame) * config.HopSize);
        samples = samples[trimStartSample..(trimStartSample + trimLength)];
		ApplyAutoGain(samples, f0, sourcePeak, config.F0Floor);

        var spectrumSize = config.FftSize / 2 + 1;
        var sourceSpectrum = new double[f0.Length * spectrumSize];
        var sourceAperiodicity = new double[f0.Length * spectrumSize];
        analyze(ref config, samples, samples.Length, f0, f0.Length, sourceSpectrum, sourceAperiodicity);

        var sourceFrames = sourceEndFrame - sourceStartFrame;
        var destinationFrames = Math.Max(2, (int)Math.Ceiling(unit.RequiredLengthMs / config.FrameMS));
        var destinationF0 = new double[destinationFrames];
        var destinationSpectrum = new double[destinationFrames * spectrumSize];
        var destinationAperiodicity = new double[destinationFrames * spectrumSize];
        var sourceLengthMS = sourceFrames * config.FrameMS;
        var consonantSpeed = Math.Pow(0.5, 1.0 - unit.ConsonantVelocity / 100.0);
        var sourceConsonantMS = Math.Min(sourceLengthMS, Math.Max(0, unit.ConsonantMs));
        var sourceVowelMS = Math.Max(0, sourceLengthMS - sourceConsonantMS);
        var destinationLengthMS = destinationFrames * config.FrameMS;
        var destinationConsonantMS = sourceConsonantMS / Math.Max(1e-6, consonantSpeed);
        var destinationVowelMS = destinationLengthMS - destinationConsonantMS;
        var vowelSpeed = destinationVowelMS > 0 ? sourceVowelMS / destinationVowelMS : 1.0;

        for (var frame = 0; frame < destinationFrames; frame++) {
            var destinationMS = frame * config.FrameMS;
            var sourceMS = destinationMS < destinationConsonantMS
                ? destinationMS * consonantSpeed
                : sourceConsonantMS + (destinationMS - destinationConsonantMS) * vowelSpeed;
            var position = Math.Clamp(sourceMS / config.FrameMS + sourceStartFrame, 0, f0.Length - 1.0);
            var left = (int)Math.Floor(position);
            var right = Math.Min(f0.Length - 1, left + 1);
            var alpha = position - left;
            destinationF0[frame] = Lerp(f0[left], f0[right], alpha);
            for (var bin = 0; bin < spectrumSize; bin++) {
                destinationSpectrum[frame * spectrumSize + bin] = Lerp(
                    sourceSpectrum[left * spectrumSize + bin], sourceSpectrum[right * spectrumSize + bin], alpha);
                destinationAperiodicity[frame * spectrumSize + bin] = Lerp(
                    sourceAperiodicity[left * spectrumSize + bin], sourceAperiodicity[right * spectrumSize + bin], alpha);
            }
        }

        var p0 = Math.Max(0, (int)Math.Round(unit.PositionMs / config.FrameMS));
        var p4 = Math.Max(p0 + 2, (int)Math.Round((unit.PositionMs + unit.LengthMs) / config.FrameMS));
        return new Segment {
            F0 = destinationF0,
            Spectrum = destinationSpectrum,
            Aperiodicity = destinationAperiodicity,
            SkipFrames = (int)Math.Round(unit.SkipMs / config.FrameMS),
            P0 = p0,
            P1 = Math.Max(p0 + 1, (int)Math.Round((unit.PositionMs + unit.FadeInMs) / config.FrameMS)),
            P3 = Math.Min(p4 - 1, (int)Math.Round((unit.PositionMs + unit.LengthMs - unit.FadeOutMs) / config.FrameMS)),
            P4 = p4,
        };
    }

    private static double[] Estimate(float[] samples, AnalysisConfig config, EstimateF0 estimate) {
        var length = estimate(samples, samples.Length, config.Fs, config.FrameMS, 0, out var pointer);
        if (length <= 0 || pointer == IntPtr.Zero) throw new InvalidOperationException("F0 returned no frames");
        try {
            var result = new double[length];
            Marshal.Copy(pointer, result, 0, length);
            return result;
        } finally {
            Marshal.FreeCoTaskMem(pointer);
        }
    }

	private static void ApplyFRQ(string path, double[] f0, int hopSize, double f0Floor) {
        if (string.IsNullOrWhiteSpace(path) || !File.Exists(path)) return;
        using var reader = new BinaryReader(File.OpenRead(path));
        if (new string(reader.ReadChars(8)) != "FREQ0003") return;
        var frqHop = reader.ReadInt32();
        reader.ReadDouble();
        reader.ReadBytes(16);
        var count = reader.ReadInt32();
        var values = new double[count];
        for (var i = 0; i < count; i++) {
            values[i] = reader.ReadDouble();
            reader.ReadDouble();
        }
        var ratio = (double)hopSize / frqHop;
        for (var frame = 0; frame < f0.Length; frame++) {
            var start = Math.Min(values.Length - 1, (int)Math.Floor(frame * ratio));
            var end = Math.Min(values.Length - 1, (int)Math.Ceiling((frame + 1) * ratio));
			var voiced = values[start..(end + 1)].Where(value => value > f0Floor).ToArray();
            f0[frame] = voiced.Length == 0 ? 0 : voiced.Average();
        }
    }

	private static void ApplyAutoGain(float[] samples, double[] f0, float sourcePeak, double f0Floor) {
		var segmentPeak = samples.Select(Math.Abs).DefaultIfEmpty(0).Max();
		if (segmentPeak < 1e-3f) return;
		var voicedRatio = f0.Count(value => value > f0Floor) / (double)Math.Max(1, f0.Length);
		var weight = 1.0 / (1.0 + Math.Exp(5.0 - 10.0 * voicedRatio));
		var targetPeak = segmentPeak * weight + sourcePeak * (1.0 - weight);
        var gain = Math.Pow(0.5 / targetPeak, 0.86);
        for (var i = 0; i < samples.Length; i++) samples[i] = (float)(samples[i] * gain);
    }

    private static double Lerp(double left, double right, double alpha) => left * (1 - alpha) + right * alpha;
	private static T Load<T>(IntPtr library, string name) where T : Delegate {
		try {
			return Marshal.GetDelegateForFunctionPointer<T>(NativeLibrary.GetExport(library, name));
		} catch (EntryPointNotFoundException error) {
			throw new InvalidDataException(
				"worldline-v2 requires a current OpenUtau worldline library; the bundled OpenUtau 0.1.565 library is too old",
				error);
		}
	}
}
