using System.Collections.Concurrent;
using System.Runtime.InteropServices;

namespace UtauTTS.WorldlineBridge;

// OpenUtauのCLASSIC＋worldline＋convergence接続を依存なしで再現する。
// faithful-phaseはSharpWavtool相当、旧方式は比較用の正規化相関を使う。
internal static class ClassicWorldline {
    private const int MaxResampleWorkers = 16;

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate int Resample(IntPtr request, out IntPtr output);

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate int FaithfulMix(
        IntPtr samples, int sampleCount, IntPtr sampleOffsets, IntPtr sampleLengths,
        IntPtr starts, IntPtr skips, IntPtr visibleLengths, IntPtr envelopeX,
        IntPtr envelopeY, int unitCount, int sampleRate, IntPtr result,
        int resultLength, IntPtr errorOutput, int errorCapacity);

    private sealed record ResampleWorker(IntPtr Library, Resample Resample);

    private sealed class Segment {
        public required Unit Unit;
        public required float[] Samples;
        public int Position;
        public int Skip;
        public int Correction;

        public int VisibleLength(int fs) {
            if (Unit.Envelope.Length >= 5) {
                return Math.Max(0, Samples.Length - Skip);
            }
            var requested = (int)Math.Round(Unit.LengthMs * fs / 1000.0);
            return Math.Max(0, Math.Min(Samples.Length - Skip, requested));
        }

        public int End(int fs) => Position + Correction + VisibleLength(fs);

        public double SampleAt(int global, int fs, int candidateCorrection = int.MinValue) {
            var correction = candidateCorrection == int.MinValue ? Correction : candidateCorrection;
            var local = global - Position - correction + Skip;
            if (local < 0 || local < Skip || local >= Samples.Length) return 0;
            return Samples[local] * Envelope(local - Skip, fs);
        }

        private double Envelope(int visible, int fs) {
            var length = VisibleLength(fs);
            if (visible < 0 || visible >= length) return 0;
            if (Unit.Envelope.Length >= 5) {
                var sample = visible + Skip;
                var shift = -Unit.Envelope[0].XMs;
                var next = 0;
                while (next < Unit.Envelope.Length && sample >
                    (Unit.Envelope[next].XMs + shift) * fs / 1000.0 + Skip) {
                    next++;
                }
                if (next == 0) return Unit.Envelope[0].Y;
                if (next >= Unit.Envelope.Length) return Unit.Envelope[^1].Y;
                var left = Unit.Envelope[next - 1];
                var right = Unit.Envelope[next];
                var leftSample = (left.XMs + shift) * fs / 1000.0 + Skip;
                var rightSample = (right.XMs + shift) * fs / 1000.0 + Skip;
                if (leftSample >= rightSample) return left.Y;
                return left.Y + (right.Y - left.Y) * (sample - leftSample) / (rightSample - leftSample);
            }
            var fadeIn = Math.Max(1, (int)Math.Round(Unit.FadeInMs * fs / 1000.0));
            var fadeOut = Math.Max(1, (int)Math.Round(Unit.FadeOutMs * fs / 1000.0));
            var gain = 1.0;
            if (visible < fadeIn) gain *= SmoothStep(visible / (double)fadeIn);
            if (visible >= length - fadeOut) gain *= SmoothStep((length - visible - 1) / (double)fadeOut);
            return gain;
        }
    }

    public static void Render(IntPtr library, Manifest manifest) {
        if (manifest.SampleRate != 44100) {
            throw new InvalidDataException($"OpenUtau classic worldline currently requires 44100 Hz; got {manifest.SampleRate} Hz");
        }
        var gpu = string.Equals(manifest.Engine,
            "classic-worldline-faithful-gpu", StringComparison.OrdinalIgnoreCase);
        Segment[] segments;
        if (gpu) {
            segments = ResampleUnitsIsolated(manifest);
        } else {
            var resample = Load<Resample>(library, "Resample");
            segments = manifest.Units.Select(unit => RenderSegment(resample, unit, manifest)).ToArray();
        }

        var faithfulPhase = string.Equals(manifest.Engine,
            "classic-worldline-faithful-phase", StringComparison.OrdinalIgnoreCase);
        if (faithfulPhase) {
            ApplySharpWavtoolConvergence(segments, manifest);
        } else {
            for (var index = 1; index < segments.Length; index++) {
                segments[index].Correction = FindConvergenceCorrection(
                    segments[index - 1], segments[index], manifest);
            }
        }

        var length = segments.Max(segment => segment.End(manifest.SampleRate));
        if (gpu) {
            var gpuMixed = MixGPU(segments, manifest, Math.Max(1, length));
            Program.WritePCM16(manifest.OutputPath, manifest.SampleRate, gpuMixed);
            return;
        }
        var mixed = new float[Math.Max(1, length)];
        foreach (var segment in segments) {
            var start = segment.Position + segment.Correction;
            var visibleLength = segment.VisibleLength(manifest.SampleRate);
            for (var visible = 0; visible < visibleLength; visible++) {
                var output = start + visible;
                if (output < 0 || output >= mixed.Length) continue;
                mixed[output] += (float)segment.SampleAt(output, manifest.SampleRate);
            }
        }
        Program.WritePCM16(manifest.OutputPath, manifest.SampleRate, mixed);
    }

    private static Segment RenderSegment(Resample resample, Unit unit, Manifest manifest) => new() {
        Unit = unit,
        Samples = ResampleUnit(resample, unit, manifest),
        Position = (int)Math.Round(unit.PositionMs * manifest.SampleRate / 1000.0),
        Skip = (int)Math.Round(unit.SkipMs * manifest.SampleRate / 1000.0),
    };

    // ResampleはFFT状態を共有するため、DLLコピーごとにワーカーを分離する。
    private static Segment[] ResampleUnitsIsolated(Manifest manifest) {
        var workerCount = Math.Min(manifest.Units.Length,
            Math.Min(Environment.ProcessorCount, MaxResampleWorkers));
        var directory = Path.Combine(Path.GetTempPath(), "utautts-worldline-workers-" + Guid.NewGuid());
        Directory.CreateDirectory(directory);
        var workers = new ConcurrentBag<ResampleWorker>();
        try {
            for (var index = 0; index < workerCount; index++) {
                var copy = Path.Combine(directory, $"worldline-{index}.dll");
                File.Copy(Path.GetFullPath(manifest.WorldlinePath), copy);
                var library = NativeLibrary.Load(copy);
                workers.Add(new ResampleWorker(library, Load<Resample>(library, "Resample")));
            }
            return manifest.Units.AsParallel().AsOrdered()
                .WithDegreeOfParallelism(workerCount)
                .Select(unit => {
                    ResampleWorker? worker;
                    while (!workers.TryTake(out worker)) Thread.Yield();
                    try {
                        return RenderSegment(worker.Resample, unit, manifest);
                    } finally {
                        workers.Add(worker);
                    }
                }).ToArray();
        } finally {
            foreach (var worker in workers) NativeLibrary.Free(worker.Library);
            Directory.Delete(directory, true);
        }
    }

    private static float[] MixGPU(Segment[] segments, Manifest manifest, int resultLength) {
        if (string.IsNullOrWhiteSpace(manifest.GpuPath)) {
            throw new InvalidDataException("faithful GPU renderer requires gpu_path");
        }
        if (segments.Any(segment => segment.Unit.Envelope.Length != 5)) {
            throw new InvalidDataException("faithful GPU mixer requires five-point envelopes");
        }
        var sampleCount = segments.Sum(segment => segment.Samples.Length);
        var samples = new float[sampleCount];
        var offsets = new int[segments.Length];
        var lengths = new int[segments.Length];
        var starts = new int[segments.Length];
        var skips = new int[segments.Length];
        var visibleLengths = new int[segments.Length];
        var envelopeX = new double[segments.Length * 5];
        var envelopeY = new double[segments.Length * 5];
        var offset = 0;
        for (var unit = 0; unit < segments.Length; unit++) {
            var segment = segments[unit];
            offsets[unit] = offset;
            lengths[unit] = segment.Samples.Length;
            starts[unit] = segment.Position + segment.Correction;
            skips[unit] = segment.Skip;
            visibleLengths[unit] = segment.VisibleLength(manifest.SampleRate);
            segment.Samples.CopyTo(samples, offset);
            offset += segment.Samples.Length;
            for (var point = 0; point < 5; point++) {
                envelopeX[unit * 5 + point] = segment.Unit.Envelope[point].XMs;
                envelopeY[unit * 5 + point] = segment.Unit.Envelope[point].Y;
            }
        }

        var result = new float[resultLength];
        var error = new byte[512];
        var arrays = new Array[] { samples, offsets, lengths, starts, skips,
            visibleLengths, envelopeX, envelopeY, result, error };
        var pins = arrays.Select(array => GCHandle.Alloc(array, GCHandleType.Pinned)).ToArray();
        var gpuLibrary = IntPtr.Zero;
        try {
            gpuLibrary = NativeLibrary.Load(Path.GetFullPath(manifest.GpuPath));
            var mix = Marshal.GetDelegateForFunctionPointer<FaithfulMix>(
                NativeLibrary.GetExport(gpuLibrary, "UtauTTSGPUFaithfulMix"));
            var ok = mix(
                pins[0].AddrOfPinnedObject(), samples.Length,
                pins[1].AddrOfPinnedObject(), pins[2].AddrOfPinnedObject(),
                pins[3].AddrOfPinnedObject(), pins[4].AddrOfPinnedObject(),
                pins[5].AddrOfPinnedObject(), pins[6].AddrOfPinnedObject(),
                pins[7].AddrOfPinnedObject(), segments.Length, manifest.SampleRate,
                pins[8].AddrOfPinnedObject(), result.Length,
                pins[9].AddrOfPinnedObject(), error.Length);
            if (ok == 0) {
                var end = Array.IndexOf(error, (byte)0);
                if (end < 0) end = error.Length;
                throw new InvalidOperationException("CUDA faithful mix failed: " +
                    System.Text.Encoding.UTF8.GetString(error, 0, end));
            }
            return result;
        } finally {
            if (gpuLibrary != IntPtr.Zero) NativeLibrary.Free(gpuLibrary);
            foreach (var pin in pins) pin.Free();
        }
    }

    private static float[] ResampleUnit(Resample resample, Unit unit, Manifest manifest) {
        var (sampleRate, samples) = Program.ReadPCM16(unit.Source);
        var tempo = unit.Tempo > 0 ? unit.Tempo : 120;
        var pitchFrameMs = 60000.0 / tempo * 5.0 / 480.0;
        var pitchLengthMs = unit.PitchLengthMs > 0 ? unit.PitchLengthMs : unit.RequiredLengthMs;
        var bendCount = Math.Max(2, (int)Math.Ceiling(pitchLengthMs / pitchFrameMs));
        var bends = new int[bendCount];
        for (var frame = 0; frame < bends.Length; frame++) {
            var timeMs = unit.PitchStartMs + frame * pitchFrameMs;
            var target = CurveAt(manifest.F0Curve, timeMs, 10.0);
            var baseF0 = 440.0 * Math.Pow(2, (unit.Tone - 69) / 12.0);
            bends[frame] = Math.Clamp((int)Math.Round(1200 * Math.Log2(target / baseF0)), -2048, 2047);
        }
        var frq = string.IsNullOrWhiteSpace(unit.FrqPath) || !File.Exists(unit.FrqPath)
            ? null : File.ReadAllBytes(unit.FrqPath);
        var samplePin = GCHandle.Alloc(samples, GCHandleType.Pinned);
        var bendPin = GCHandle.Alloc(bends, GCHandleType.Pinned);
        GCHandle? frqPin = frq == null ? null : GCHandle.Alloc(frq, GCHandleType.Pinned);
        var requestMemory = Marshal.AllocHGlobal(Marshal.SizeOf<SynthRequest>());
        try {
            var request = new SynthRequest {
                sample_fs = sampleRate, sample_length = samples.Length,
                sample = samplePin.AddrOfPinnedObject(),
                frq_length = frq?.Length ?? 0, frq = frqPin?.AddrOfPinnedObject() ?? IntPtr.Zero,
                tone = unit.Tone, con_vel = unit.ConsonantVelocity,
                offset = unit.OffsetMs, required_length = unit.RequiredLengthMs,
                consonant = unit.ConsonantMs, cut_off = unit.CutoffMs,
                volume = unit.Volume, modulation = unit.Modulation, tempo = tempo,
                pitch_bend_length = bends.Length, pitch_bend = bendPin.AddrOfPinnedObject(),
                flag_P = 86, flag_Mv = 100,
            };
            Marshal.StructureToPtr(request, requestMemory, false);
            var sampleCount = resample(requestMemory, out var output);
            if (sampleCount <= 0 || output == IntPtr.Zero) {
                throw new InvalidOperationException($"worldline Resample returned no audio for {unit.Source}");
            }
            try {
                var result = new float[sampleCount];
                Marshal.Copy(output, result, 0, sampleCount);
                return result;
            } finally {
                Marshal.FreeCoTaskMem(output);
            }
        } finally {
            Marshal.FreeHGlobal(requestMemory);
            bendPin.Free();
            samplePin.Free();
            if (frqPin.HasValue) frqPin.Value.Free();
        }
    }

    private static int FindConvergenceCorrection(Segment previous, Segment current, Manifest manifest) {
        var fs = manifest.SampleRate;
        var overlapStart = Math.Max(previous.Position + previous.Correction, current.Position);
        var overlapEnd = Math.Min(previous.End(fs), current.Position + Math.Max(256,
            (int)Math.Round(current.Unit.FadeInMs * fs / 1000.0)));
        if (overlapEnd - overlapStart < 128) return 0;
        var f0 = CurveAt(manifest.F0Curve, current.Unit.PositionMs + current.Unit.FadeInMs * 0.5, 10.0);
        if (!double.IsFinite(f0) || f0 < 40) return 0;
        var radius = Math.Clamp((int)Math.Round(fs / f0 * 0.5), 1, 550);
        var bestShift = 0;
        var bestScore = double.NegativeInfinity;
        for (var shift = -radius; shift <= radius; shift++) {
            double cross = 0, leftEnergy = 0, rightEnergy = 0;
            for (var sample = overlapStart; sample < overlapEnd; sample += 2) {
                var left = previous.SampleAt(sample, fs);
                var right = current.SampleAt(sample, fs, shift);
                cross += left * right;
                leftEnergy += left * left;
                rightEnergy += right * right;
            }
            if (leftEnergy < 1e-8 || rightEnergy < 1e-8) continue;
            var score = cross / Math.Sqrt(leftEnergy * rightEnergy);
            if (score > bestScore) {
                bestScore = score;
                bestShift = shift;
            }
        }
        return bestScore > 0 ? bestShift : 0;
    }

    private readonly record struct PhasePoint(double F0, double Phase, bool Valid);

    private static void ApplySharpWavtoolConvergence(Segment[] segments, Manifest manifest) {
        if (segments.Length < 2) return;
        var heads = new PhasePoint[segments.Length];
        var tails = new PhasePoint[segments.Length];
        for (var index = 0; index < segments.Length; index++) {
            heads[index] = MeasureEnvelopePhase(segments[index], manifest, true);
            tails[index] = MeasureEnvelopePhase(segments[index], manifest, false);
        }
        for (var index = 1; index < segments.Length; index++) {
            var previous = tails[index - 1];
            var current = heads[index];
            if (!previous.Valid || !current.Valid || current.F0 <= 0) continue;
            var lastCorrectionAngle = segments[index - 1].Correction * 2 * Math.PI /
                manifest.SampleRate * current.F0;
            var difference = current.Phase - (previous.Phase - lastCorrectionAngle);
            difference %= 2 * Math.PI;
            if (difference < 0) difference += 2 * Math.PI;
            if (Math.Abs(difference - 2 * Math.PI) < difference) difference -= 2 * Math.PI;
            segments[index].Correction = (int)(difference / (2 * Math.PI) *
                manifest.SampleRate / current.F0);
        }
    }

    private static PhasePoint MeasureEnvelopePhase(Segment segment, Manifest manifest, bool head) {
        if (segment.Unit.Envelope.Length < 5 || segment.Samples.Length < 3) return default;
        var envelope = segment.Unit.Envelope;
        var centerMs = head
            ? (envelope[0].XMs + envelope[1].XMs) * 0.5
            : (envelope[3].XMs + envelope[4].XMs) * 0.5;
        var shiftMs = -envelope[0].XMs;
        var center = (int)((centerMs + shiftMs) * manifest.SampleRate / 1000.0 + segment.Skip);
        var start = Math.Max(center - 440, 0);
        var length = Math.Min(880, segment.Samples.Length - start);
        if (length < 3) return default;

        var globalCenter = segment.Position - segment.Skip + start + length * 0.5;
        var timeMs = globalCenter * 1000.0 / manifest.SampleRate;
        var pitchFrameMs = 60000.0 / (segment.Unit.Tempo > 0 ? segment.Unit.Tempo : 120) * 5.0 / 480.0;
        var pitchTimeMs = Math.Round(timeMs / pitchFrameMs) * pitchFrameMs;
        var f0 = CurveAt(manifest.F0Curve, pitchTimeMs, 10.0);
        if (!double.IsFinite(f0) || f0 <= 0 || f0 >= manifest.SampleRate * 0.5) return default;

        var filtered = ZeroPhasePeak(segment.Samples.AsSpan(start, length), manifest.SampleRate, f0, 5);
        var middle = filtered.Length / 2;
        var left = middle - 1;
        while (left > 0 && !(filtered[left] > filtered[left - 1] && filtered[left] >= filtered[left + 1])) left--;
        var right = middle;
        while (right < filtered.Length - 1 && !(filtered[right] >= filtered[right - 1] && filtered[right] > filtered[right + 1])) right++;
        if (left >= right || left <= 0 || right >= filtered.Length - 1) return default;
        var actualF0 = manifest.SampleRate / (double)(right - left);
        if (Math.Abs(f0 - actualF0) > 0.25 * f0) return default;
        var cycles = (segment.Position - segment.Skip + start + (left + right) * 0.5) /
            manifest.SampleRate * f0;
        var phase = 2 * Math.PI * (Math.Round(cycles) - cycles);
        return new PhasePoint(f0, phase, true);
    }

    private static double[] ZeroPhasePeak(ReadOnlySpan<float> input, int sampleRate, double f0, double q) {
        var omega = 2 * Math.PI * f0 / sampleRate;
        var alpha = Math.Sin(omega) / (2 * q);
        var denominator = 1 + alpha;
        var b0 = alpha / denominator;
        var b2 = -b0;
        var a1 = -2 * Math.Cos(omega) / denominator;
        var a2 = (1 - alpha) / denominator;
        var forward = FilterPeak(input, b0, b2, a1, a2);
        Array.Reverse(forward);
        var backward = FilterPeak(forward, b0, b2, a1, a2);
        Array.Reverse(backward);
        return backward;
    }

    private static double[] FilterPeak(ReadOnlySpan<float> input, double b0, double b2, double a1, double a2) {
        var output = new double[input.Length];
        double z1 = 0, z2 = 0;
        for (var index = 0; index < input.Length; index++) {
            var value = b0 * input[index] + z1;
            z1 = z2 - a1 * value;
            z2 = b2 * input[index] - a2 * value;
            output[index] = value;
        }
        return output;
    }

    private static double[] FilterPeak(ReadOnlySpan<double> input, double b0, double b2, double a1, double a2) {
        var output = new double[input.Length];
        double z1 = 0, z2 = 0;
        for (var index = 0; index < input.Length; index++) {
            var value = b0 * input[index] + z1;
            z1 = z2 - a1 * value;
            z2 = b2 * input[index] - a2 * value;
            output[index] = value;
        }
        return output;
    }

    private static double CurveAt(double[] curve, double timeMs, double frameMs) {
        if (curve.Length == 0) return 220;
        var position = Math.Max(0, timeMs) / frameMs;
        var left = Math.Min(curve.Length - 1, (int)Math.Floor(position));
        var right = Math.Min(curve.Length - 1, left + 1);
        var alpha = Math.Clamp(position - left, 0, 1);
        return curve[left] * (1 - alpha) + curve[right] * alpha;
    }

    private static double SmoothStep(double value) {
        value = Math.Clamp(value, 0, 1);
        return value * value * (3 - 2 * value);
    }

    private static T Load<T>(IntPtr library, string name) where T : Delegate {
        try {
            return Marshal.GetDelegateForFunctionPointer<T>(NativeLibrary.GetExport(library, name));
        } catch (EntryPointNotFoundException error) {
            throw new InvalidDataException("worldline library does not expose the classic Resample entry point", error);
        }
    }
}
