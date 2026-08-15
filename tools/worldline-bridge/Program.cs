using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace UtauTTS.WorldlineBridge;

internal sealed class Manifest {
    [JsonPropertyName("engine")] public string Engine { get; set; } = "";
    [JsonPropertyName("worldline_path")] public string WorldlinePath { get; set; } = "";
    [JsonPropertyName("gpu_path")] public string GpuPath { get; set; } = "";
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
	[JsonPropertyName("pitch_start_ms")] public double PitchStartMs { get; set; }
	[JsonPropertyName("pitch_length_ms")] public double PitchLengthMs { get; set; }
	[JsonPropertyName("volume")] public double Volume { get; set; } = 100;
	[JsonPropertyName("modulation")] public double Modulation { get; set; }
	[JsonPropertyName("tempo")] public double Tempo { get; set; } = 120;
	[JsonPropertyName("envelope")] public EnvelopePoint[] Envelope { get; set; } = [];
}

internal sealed class EnvelopePoint {
	[JsonPropertyName("x_ms")] public double XMs { get; set; }
	[JsonPropertyName("y")] public double Y { get; set; }
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
			if (string.Equals(manifest.Engine, "classic-worldline-faithful", StringComparison.OrdinalIgnoreCase) ||
				string.Equals(manifest.Engine, "classic-worldline-faithful-gpu", StringComparison.OrdinalIgnoreCase)) {
				ClassicWorldline.Render(library, manifest);
				return;
			}
			throw new InvalidDataException($"unknown engine: {manifest.Engine}");
        } finally {
            NativeLibrary.Free(library);
        }
    }

	internal static (int sampleRate, double[] samples) ReadPCM16(string path) {
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

	internal static void WritePCM16(string path, int sampleRate, float[] samples) {
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
