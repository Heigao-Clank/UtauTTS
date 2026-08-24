using System.Runtime.InteropServices;

namespace UtauTTS.WorldlineBridge;

// OpenUtau 0.1.565のPhraseSynth APIでフレーズ全体を一度に合成する。
internal static class WorldlineR {
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate IntPtr PhraseSynthNew();

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate void PhraseSynthDelete(IntPtr phraseSynth);

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate void PhraseSynthAddRequest(
        IntPtr phraseSynth, IntPtr request, double positionMs, double skipMs,
        double lengthMs, double fadeInMs, double fadeOutMs, LogCallback logCallback);

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate void PhraseSynthSetCurves(
        IntPtr phraseSynth, IntPtr f0, IntPtr gender, IntPtr tension,
        IntPtr breathiness, IntPtr voicing, int length, LogCallback logCallback);

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    private delegate int PhraseSynthSynth(IntPtr phraseSynth, out IntPtr output, LogCallback logCallback);

    [UnmanagedFunctionPointer(CallingConvention.StdCall, CharSet = CharSet.Ansi)]
    private delegate void LogCallback([MarshalAs(UnmanagedType.LPStr)] string message);

    private static readonly LogCallback Log = _ => { };

    public static void Render(IntPtr library, Manifest manifest) {
        if (manifest.SampleRate != 44100) {
            throw new InvalidDataException(
                $"OpenUtau WORLDLINE-R 0.1.565 requires 44100 Hz; got {manifest.SampleRate} Hz");
        }
        var create = Load<PhraseSynthNew>(library, "PhraseSynthNew");
        var destroy = Load<PhraseSynthDelete>(library, "PhraseSynthDelete");
        var add = Load<PhraseSynthAddRequest>(library, "PhraseSynthAddRequest");
        var setCurves = Load<PhraseSynthSetCurves>(library, "PhraseSynthSetCurves");
        var synthesize = Load<PhraseSynthSynth>(library, "PhraseSynthSynth");

        var phraseSynth = create();
        if (phraseSynth == IntPtr.Zero) {
            throw new InvalidOperationException("worldline PhraseSynthNew returned null");
        }
        try {
            foreach (var unit in manifest.Units) {
                AddUnit(phraseSynth, add, unit);
            }
            SetCurves(phraseSynth, setCurves, manifest.F0Curve);
            var sampleCount = synthesize(phraseSynth, out var output, Log);
            if (sampleCount <= 0 || output == IntPtr.Zero) {
                throw new InvalidOperationException("worldline PhraseSynthSynth returned no audio");
            }
            try {
                var samples = new float[sampleCount];
                Marshal.Copy(output, samples, 0, sampleCount);
                Program.WritePCM16(manifest.OutputPath, manifest.SampleRate, samples);
            } finally {
                Marshal.FreeCoTaskMem(output);
            }
        } finally {
            destroy(phraseSynth);
        }
    }

    private static void AddUnit(IntPtr phraseSynth, PhraseSynthAddRequest add, Unit unit) {
        var (sampleRate, samples) = Program.ReadPCM16(unit.Source);
        ValidateInputRegion(unit, sampleRate, samples.Length);
        var frq = string.IsNullOrWhiteSpace(unit.FrqPath) || !File.Exists(unit.FrqPath)
            ? null : File.ReadAllBytes(unit.FrqPath);
        var pitches = new int[2];
        var samplePin = GCHandle.Alloc(samples, GCHandleType.Pinned);
        var pitchPin = GCHandle.Alloc(pitches, GCHandleType.Pinned);
        GCHandle? frqPin = frq == null ? null : GCHandle.Alloc(frq, GCHandleType.Pinned);
        var requestMemory = Marshal.AllocHGlobal(Marshal.SizeOf<SynthRequest>());
        try {
            var request = new SynthRequest {
                sample_fs = sampleRate,
                sample_length = samples.Length,
                sample = samplePin.AddrOfPinnedObject(),
                frq_length = frq?.Length ?? 0,
                frq = frqPin?.AddrOfPinnedObject() ?? IntPtr.Zero,
                tone = unit.Tone,
                con_vel = unit.ConsonantVelocity,
                offset = unit.OffsetMs,
                required_length = unit.RequiredLengthMs,
                consonant = unit.ConsonantMs,
                cut_off = unit.CutoffMs,
                volume = unit.Volume,
                modulation = unit.Modulation,
                tempo = unit.Tempo > 0 ? unit.Tempo : 120,
                pitch_bend_length = pitches.Length,
                pitch_bend = pitchPin.AddrOfPinnedObject(),
                flag_P = 86,
                flag_Mv = 100,
            };
            Marshal.StructureToPtr(request, requestMemory, false);
            add(phraseSynth, requestMemory, unit.PositionMs, unit.SkipMs,
                unit.LengthMs, unit.FadeInMs, unit.FadeOutMs, Log);
        } finally {
            Marshal.FreeHGlobal(requestMemory);
            pitchPin.Free();
            samplePin.Free();
            if (frqPin.HasValue) frqPin.Value.Free();
        }
    }

    private static void ValidateInputRegion(Unit unit, int sampleRate, int sampleCount) {
        const double frameMs = 10;
        var totalMs = sampleCount * 1000.0 / sampleRate;
        var inputLengthMs = unit.CutoffMs < 0
            ? -unit.CutoffMs
            : totalMs - unit.OffsetMs - unit.CutoffMs;
        if (unit.OffsetMs < 0 || unit.OffsetMs + inputLengthMs > totalMs + 0.1) {
            throw new InvalidDataException($"oto range exceeds source audio: {unit.Source}");
        }
        var startFrame = (int)(unit.OffsetMs / frameMs);
        var frameCount = (int)Math.Ceiling((unit.OffsetMs + inputLengthMs) / frameMs) - startFrame;
        var maximumFrames = (int)Math.Ceiling(totalMs / frameMs);
        frameCount = Math.Min(frameCount, maximumFrames - startFrame);
        if (frameCount <= 0) {
            throw new InvalidDataException($"oto cutoff is before offset: {unit.Source}");
        }
        if (unit.RequiredLengthMs <= 0 || unit.LengthMs <= 0 ||
            unit.SkipMs + unit.LengthMs > unit.RequiredLengthMs + frameMs + 0.1) {
            throw new InvalidDataException($"WORLDLINE-R timing exceeds remapped source: {unit.Source}");
        }
    }

    private static void SetCurves(
        IntPtr phraseSynth, PhraseSynthSetCurves setCurves, double[] f0) {
        var gender = Enumerable.Repeat(0.5, f0.Length).ToArray();
        var tension = Enumerable.Repeat(0.5, f0.Length).ToArray();
        var breathiness = Enumerable.Repeat(0.5, f0.Length).ToArray();
        var voicing = Enumerable.Repeat(1.0, f0.Length).ToArray();
        var arrays = new[] { f0, gender, tension, breathiness, voicing };
        var pins = arrays.Select(array => GCHandle.Alloc(array, GCHandleType.Pinned)).ToArray();
        try {
            setCurves(phraseSynth,
                pins[0].AddrOfPinnedObject(), pins[1].AddrOfPinnedObject(),
                pins[2].AddrOfPinnedObject(), pins[3].AddrOfPinnedObject(),
                pins[4].AddrOfPinnedObject(), f0.Length, Log);
        } finally {
            foreach (var pin in pins) pin.Free();
        }
    }

    private static T Load<T>(IntPtr library, string name) where T : Delegate {
        try {
            return Marshal.GetDelegateForFunctionPointer<T>(NativeLibrary.GetExport(library, name));
        } catch (EntryPointNotFoundException error) {
            throw new InvalidDataException(
                $"worldline library does not expose the WORLDLINE-R entry point {name}", error);
        }
    }
}
