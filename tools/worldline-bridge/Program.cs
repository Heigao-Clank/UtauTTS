using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace UtauTTS.WorldlineBridge;

internal sealed class Manifest {
    [JsonPropertyName("worldline_path")] public string WorldlinePath { get; set; } = "";
    [JsonPropertyName("output_path")] public string OutputPath { get; set; } = "";
    [JsonPropertyName("sample_rate")] public int SampleRate { get; set; }
    [JsonPropertyName("f0_curve")] public double[] F0Curve { get; set; } = [];
    [JsonPropertyName("units")] public Unit[] Units { get; set; } = [];
}

internal sealed class Unit {
    [JsonPropertyName("source")] public string Source { get; set; } = "";
    [JsonPropertyName("frq_path")] public string FrqPath { get; set; } = "";
    [JsonPropertyName("position_ms")] public double PositionMs { get; set; }
    [JsonPropertyName("skip_ms")] public double SkipMs { get; set; }
    [JsonPropertyName("length_ms")] public double LengthMs { get; set; }
    [JsonPropertyName("fade_in_ms")] public double FadeInMs { get; set; }
    [JsonPropertyName("fade_out_ms")] public double FadeOutMs { get; set; }
    [JsonPropertyName("offset_ms")] public double OffsetMs { get; set; }
    [JsonPropertyName("required_length_ms")] public double RequiredLengthMs { get; set; }
    [JsonPropertyName("consonant_ms")] public double ConsonantMs { get; set; }
    [JsonPropertyName("cutoff_ms")] public double CutoffMs { get; set; }
    [JsonPropertyName("tone")] public int Tone { get; set; }
    [JsonPropertyName("consonant_velocity")] public double ConsonantVelocity { get; set; }
}

[StructLayout(LayoutKind.Sequential)]
internal struct SynthRequest {
    public int sample_fs;
    public int sample_length;
    public IntPtr sample;
    public int frq_length;
    public IntPtr frq;
    public int tone;
    public double con_vel;
    public double offset;
    public double required_length;
    public double consonant;
    public double cut_off;
    public double volume;
    public double modulation;
    public double tempo;
    public int pitch_bend_length;
    public IntPtr pitch_bend;
    public int flag_g;
    public int flag_O;
    public int flag_P;
    public int flag_Mt;
    public int flag_Mb;
    public int flag_Mv;
}

[UnmanagedFunctionPointer(CallingConvention.Cdecl)] internal delegate IntPtr PhraseNew();
[UnmanagedFunctionPointer(CallingConvention.Cdecl)] internal delegate void PhraseDelete(IntPtr phrase);
[UnmanagedFunctionPointer(CallingConvention.Cdecl)] internal delegate void PhraseAdd(
    IntPtr phrase, IntPtr request, double positionMs, double skipMs, double lengthMs,
    double fadeInMs, double fadeOutMs, IntPtr logCallback);
[UnmanagedFunctionPointer(CallingConvention.Cdecl)] internal delegate void PhraseSetCurves(
    IntPtr phrase, IntPtr f0, IntPtr gender, IntPtr tension, IntPtr breathiness,
    IntPtr voicing, int length, IntPtr logCallback);
[UnmanagedFunctionPointer(CallingConvention.Cdecl)] internal delegate int PhraseSynth(
    IntPtr phrase, out IntPtr output, IntPtr logCallback);

internal static class Program {
    public static int Main(string[] args) {
        try {
            if (args.Length != 1) throw new ArgumentException("usage: utautts-worldline-bridge MANIFEST.json");
            var manifest = JsonSerializer.Deserialize<Manifest>(File.ReadAllText(args[0]))
                ?? throw new InvalidDataException("invalid manifest");
            Render(manifest);
            return 0;
        } catch (Exception error) {
            Console.Error.WriteLine(error);
            return 1;
        }
    }

    private static void Render(Manifest manifest) {
        if (manifest.Units.Length == 0 || manifest.F0Curve.Length < 2) {
            throw new InvalidDataException("manifest has no synthesis data");
        }
        var library = NativeLibrary.Load(Path.GetFullPath(manifest.WorldlinePath));
        try {
            var create = Load<PhraseNew>(library, "PhraseSynthNew");
            var delete = Load<PhraseDelete>(library, "PhraseSynthDelete");
            var add = Load<PhraseAdd>(library, "PhraseSynthAddRequest");
            var setCurves = Load<PhraseSetCurves>(library, "PhraseSynthSetCurves");
            var synth = Load<PhraseSynth>(library, "PhraseSynthSynth");
            var phrase = create();
            if (phrase == IntPtr.Zero) throw new InvalidOperationException("PhraseSynthNew returned null");
            try {
                foreach (var unit in manifest.Units) AddUnit(phrase, add, unit, manifest.SampleRate);
                using var curves = new PinnedCurves(manifest.F0Curve);
                setCurves(phrase, curves.F0, curves.Gender, curves.Tension,
                    curves.Breathiness, curves.Voicing, manifest.F0Curve.Length, IntPtr.Zero);
                var length = synth(phrase, out var output, IntPtr.Zero);
                if (length <= 0 || output == IntPtr.Zero) throw new InvalidOperationException("PhraseSynthSynth returned no audio");
                try {
                    var samples = new float[length];
                    Marshal.Copy(output, samples, 0, length);
                    WritePCM16(manifest.OutputPath, manifest.SampleRate, samples);
                } finally {
                    Marshal.FreeCoTaskMem(output);
                }
            } finally {
                delete(phrase);
            }
        } finally {
            NativeLibrary.Free(library);
        }
    }

    private static void AddUnit(IntPtr phrase, PhraseAdd add, Unit unit, int expectedSampleRate) {
        var (sampleRate, samples) = ReadPCM16(unit.Source);
        if (sampleRate != expectedSampleRate) {
            throw new InvalidDataException($"{unit.Source} is {sampleRate} Hz; expected {expectedSampleRate} Hz");
        }
        var pitchBend = new int[Math.Max(2, (int)Math.Ceiling(unit.RequiredLengthMs / 10.0))];
        var frq = string.IsNullOrWhiteSpace(unit.FrqPath) ? null : File.ReadAllBytes(unit.FrqPath);
        var samplePin = GCHandle.Alloc(samples, GCHandleType.Pinned);
        var pitchPin = GCHandle.Alloc(pitchBend, GCHandleType.Pinned);
        GCHandle? frqPin = frq == null ? null : GCHandle.Alloc(frq, GCHandleType.Pinned);
        var requestMemory = Marshal.AllocHGlobal(Marshal.SizeOf<SynthRequest>());
        try {
            var request = new SynthRequest {
                sample_fs = sampleRate, sample_length = samples.Length,
                sample = samplePin.AddrOfPinnedObject(),
                frq_length = frq?.Length ?? 0, frq = frqPin?.AddrOfPinnedObject() ?? IntPtr.Zero,
                tone = unit.Tone,
                con_vel = unit.ConsonantVelocity, offset = unit.OffsetMs,
                required_length = unit.RequiredLengthMs, consonant = unit.ConsonantMs,
                cut_off = unit.CutoffMs, volume = 100, tempo = 120,
                pitch_bend_length = pitchBend.Length, pitch_bend = pitchPin.AddrOfPinnedObject(),
                flag_P = 86, flag_Mv = 100,
            };
            Marshal.StructureToPtr(request, requestMemory, false);
            add(phrase, requestMemory, unit.PositionMs, unit.SkipMs, unit.LengthMs,
                unit.FadeInMs, unit.FadeOutMs, IntPtr.Zero);
        } finally {
            Marshal.FreeHGlobal(requestMemory);
            pitchPin.Free();
            samplePin.Free();
            if (frqPin.HasValue) frqPin.Value.Free();
        }
    }

    private static T Load<T>(IntPtr library, string name) where T : Delegate =>
        Marshal.GetDelegateForFunctionPointer<T>(NativeLibrary.GetExport(library, name));

    private sealed class PinnedCurves : IDisposable {
        private readonly GCHandle[] handles;
        public IntPtr F0 => handles[0].AddrOfPinnedObject();
        public IntPtr Gender => handles[1].AddrOfPinnedObject();
        public IntPtr Tension => handles[2].AddrOfPinnedObject();
        public IntPtr Breathiness => handles[3].AddrOfPinnedObject();
        public IntPtr Voicing => handles[4].AddrOfPinnedObject();
        public PinnedCurves(double[] f0) {
            var length = f0.Length;
            handles = new[] { f0, Fill(length, 0.5), Fill(length, 0.5), Fill(length, 0.5), Fill(length, 1) }
                .Select(values => GCHandle.Alloc(values, GCHandleType.Pinned)).ToArray();
        }
        public void Dispose() { foreach (var handle in handles) handle.Free(); }
        private static double[] Fill(int length, double value) => Enumerable.Repeat(value, length).ToArray();
    }

    private static (int sampleRate, double[] samples) ReadPCM16(string path) {
        using var stream = File.OpenRead(path);
        using var reader = new BinaryReader(stream);
        if (new string(reader.ReadChars(4)) != "RIFF") throw new InvalidDataException($"not RIFF: {path}");
        reader.ReadUInt32();
        if (new string(reader.ReadChars(4)) != "WAVE") throw new InvalidDataException($"not WAVE: {path}");
        ushort format = 0, channels = 0, bits = 0;
        int sampleRate = 0;
        byte[]? data = null;
        while (stream.Position + 8 <= stream.Length) {
            var id = new string(reader.ReadChars(4));
            var size = reader.ReadUInt32();
            var end = stream.Position + size;
            if (id == "fmt ") {
                format = reader.ReadUInt16(); channels = reader.ReadUInt16(); sampleRate = reader.ReadInt32();
                reader.ReadUInt32(); reader.ReadUInt16(); bits = reader.ReadUInt16();
            } else if (id == "data") {
                data = reader.ReadBytes(checked((int)size));
            }
            stream.Position = Math.Min(stream.Length, end + (size & 1));
        }
        if (format != 1 || channels < 1 || bits != 16 || data == null) {
            throw new InvalidDataException($"worldline bridge supports PCM16 WAV only: {path}");
        }
        var frameCount = data.Length / (channels * 2);
        var samples = new double[frameCount];
        for (var frame = 0; frame < frameCount; frame++) {
            var sum = 0.0;
            for (var channel = 0; channel < channels; channel++) {
                sum += BitConverter.ToInt16(data, (frame * channels + channel) * 2) / 32768.0;
            }
            samples[frame] = sum / channels;
        }
        return (sampleRate, samples);
    }

    private static void WritePCM16(string path, int sampleRate, float[] samples) {
        var peak = samples.Where(float.IsFinite).Select(Math.Abs).DefaultIfEmpty(0).Max();
        var scale = peak > 0.98f ? 0.98f / peak : 1f;
        Directory.CreateDirectory(Path.GetDirectoryName(Path.GetFullPath(path))!);
        using var stream = File.Create(path);
        using var writer = new BinaryWriter(stream);
        writer.Write("RIFF"u8.ToArray()); writer.Write(36 + samples.Length * 2); writer.Write("WAVE"u8.ToArray());
        writer.Write("fmt "u8.ToArray()); writer.Write(16); writer.Write((ushort)1); writer.Write((ushort)1);
        writer.Write(sampleRate); writer.Write(sampleRate * 2); writer.Write((ushort)2); writer.Write((ushort)16);
        writer.Write("data"u8.ToArray()); writer.Write(samples.Length * 2);
        foreach (var value in samples) {
            var safe = float.IsFinite(value) ? Math.Clamp(value * scale, -1, 1) : 0;
            writer.Write((short)Math.Round(safe * 32767));
        }
    }
}
