import argparse
import json
import os

import numpy as np
import pyworld as pw
import soundfile as sf


def main() -> None:
    parser = argparse.ArgumentParser(description="Extract features using WORLD vocoder")
    parser.add_argument("--manifest", required=True, help="path to manifest.jsonl")
    parser.add_argument("--audio-root", default="", help="directory containing trimmed wavs")
    parser.add_argument("--out", required=True, help="output jsonl path")
    parser.add_argument("--curve-bins", type=int, default=20, help="bins for F0/RMS curves")
    parser.add_argument("--fmin", type=float, default=60.0, help="minimum F0")
    parser.add_argument("--fmax", type=float, default=600.0, help="maximum F0")
    args = parser.parse_args()

    audio_root = args.audio_root
    if not audio_root:
        audio_root = os.path.dirname(os.path.abspath(args.manifest))

    with open(args.manifest, "r", encoding="utf-8") as manifest_file, open(
        args.out, "w", encoding="utf-8"
    ) as out_file:
        for line in manifest_file:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            wav_name = record["trimmed"]
            wav_path = os.path.join(audio_root, wav_name)
            if not os.path.exists(wav_path):
                raise FileNotFoundError(wav_path)

            y, sr = sf.read(wav_path, dtype="float64")
            if y.ndim > 1:
                y = np.mean(y, axis=1)

            f0, t = pw.dio(y, sr, f0_floor=args.fmin, f0_ceil=args.fmax)
            f0 = pw.stonemask(y, f0, t, sr)
            rms = np.sqrt(np.mean(pw.cheaptrick(y, f0, t, sr) ** 2, axis=1))

            f0_valid = f0[f0 > 0]
            f0_mean = float(np.mean(f0_valid)) if f0_valid.size else 0.0
            f0_std = float(np.std(f0_valid)) if f0_valid.size else 0.0
            f0_median = float(np.median(f0_valid)) if f0_valid.size else 0.0
            f0_q10 = float(np.quantile(f0_valid, 0.1)) if f0_valid.size else 0.0
            f0_q90 = float(np.quantile(f0_valid, 0.9)) if f0_valid.size else 0.0

            rms_mean = float(np.mean(rms)) if rms.size else 0.0
            rms_std = float(np.std(rms)) if rms.size else 0.0
            rms_median = float(np.median(rms)) if rms.size else 0.0
            rms_q10 = float(np.quantile(rms, 0.1)) if rms.size else 0.0
            rms_q90 = float(np.quantile(rms, 0.9)) if rms.size else 0.0

            f0_curve = resize_curve(f0, args.curve_bins)
            rms_curve = resize_curve(rms, args.curve_bins)

            feature_record = {
                "alias": record.get("alias"),
                "source": record.get("source"),
                "trimmed": record.get("trimmed"),
                "sample_rate": int(sr),
                "f0": f0.astype(np.float32).tolist(),
                "rms": rms.astype(np.float32).tolist(),
                "f0_curve": f0_curve.astype(np.float32).tolist(),
                "rms_curve": rms_curve.astype(np.float32).tolist(),
                "f0_mean": f0_mean,
                "f0_std": f0_std,
                "f0_median": f0_median,
                "f0_q10": f0_q10,
                "f0_q90": f0_q90,
                "rms_mean": rms_mean,
                "rms_std": rms_std,
                "rms_median": rms_median,
                "rms_q10": rms_q10,
                "rms_q90": rms_q90,
            }
            out_file.write(json.dumps(feature_record, ensure_ascii=False))
            out_file.write("\n")


def resize_curve(values: np.ndarray, bins: int) -> np.ndarray:
    if bins <= 0:
        return np.array([], dtype=np.float32)
    if values.size == 0:
        return np.zeros(bins, dtype=np.float32)
    finite_mask = np.isfinite(values)
    if not np.any(finite_mask):
        return np.zeros(bins, dtype=np.float32)
    x = np.arange(values.size, dtype=np.float64)
    xp = np.linspace(0, values.size - 1, num=bins, dtype=np.float64)
    return np.interp(xp, x, np.nan_to_num(values, nan=0.0)).astype(np.float32)


if __name__ == "__main__":
    main()
