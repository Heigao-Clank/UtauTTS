import argparse
import os

import numpy as np
import pyworld as pw
import soundfile as sf


def main() -> None:
    parser = argparse.ArgumentParser(description="Resynthesize wav with modified F0 curve")
    parser.add_argument("--input", required=True, help="input wav")
    parser.add_argument("--f0-curve", default="", help="comma-separated F0 curve values")
    parser.add_argument("--out", required=True, help="output wav")
    parser.add_argument("--fmin", type=float, default=60.0)
    parser.add_argument("--fmax", type=float, default=600.0)
    args = parser.parse_args()

    y, sr = sf.read(args.input, dtype="float64")
    if y.ndim > 1:
        y = np.mean(y, axis=1)

    f0, t = pw.dio(y, sr, f0_floor=args.fmin, f0_ceil=args.fmax)
    f0 = pw.stonemask(y, f0, t, sr)
    sp = pw.cheaptrick(y, f0, t, sr)
    ap = pw.d4c(y, f0, t, sr)

    if args.f0_curve:
        curve = [float(v) for v in args.f0_curve.split(",") if v.strip()]
        if curve:
            orig_mean = np.mean(f0[f0 > 0]) if np.any(f0 > 0) else 1.0
            if orig_mean > 0:
                bins = len(curve)
                n_frames = f0.shape[0]
                for i in range(n_frames):
                    bin_idx = int(i * bins / n_frames)
                    if bin_idx >= bins:
                        bin_idx = bins - 1
                    factor = curve[bin_idx] / orig_mean
                    factor = np.clip(factor, 0.5, 2.0)
                    f0[i] *= factor

    y_out = pw.synthesize(f0, sp, ap, sr)
    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    sf.write(args.out, y_out.astype(np.float32), sr)


if __name__ == "__main__":
    main()
