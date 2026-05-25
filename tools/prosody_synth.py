import argparse
import json
import os
import re

import numpy as np
import pyworld as pw
import soundfile as sf


def main() -> None:
    parser = argparse.ArgumentParser(description="WORLD synthesis with mean-shift F0 modulation")
    parser.add_argument("--oto", required=True)
    parser.add_argument("--plan", required=True, help="plan jsonl from predict_jsut.py")
    parser.add_argument("--out", required=True)
    parser.add_argument("--speed", type=float, default=1.0)
    parser.add_argument("--crossfade-ms", type=float, default=20.0)
    parser.add_argument("--vocoder", default="", help="HiFi-GAN checkpoint (optional)")
    args = parser.parse_args()

    plan = load_plan(args.plan)
    if len(plan) < 1:
        raise ValueError("empty plan")

    oto_dir = os.path.dirname(os.path.abspath(args.oto))

    segments = []
    seg_dur_ms = []
    seg_pf = []

    sr = None
    for item in plan:
        wav_path = item.get("file", "")
        if not os.path.exists(wav_path):
            wav_path = os.path.join(oto_dir, os.path.basename(wav_path))
        if not os.path.exists(wav_path):
            continue

        y, sr_file = sf.read(wav_path, dtype="float64")
        if y.ndim > 1:
            y = np.mean(y, axis=1)
        if sr is None:
            sr = sr_file
        if sr_file != sr:
            continue

        offset_ms = float(item.get("offset_ms", 0))
        blank_ms = float(item.get("blank_ms", 0))
        offset_s = max(0, int(offset_ms * sr / 1000))
        blank_s = int(blank_ms * sr / 1000)
        end = max(offset_s + 1, len(y) - blank_s)
        end = min(end, len(y))
        y = y[offset_s:end]

        segments.append(y)
        seg_dur_ms.append(float(item.get("target_dur_ms", 0)))
        seg_pf.append(float(item.get("pitch_factor", 1.0)))

    if not segments:
        raise ValueError("no valid segments")

    n = len(segments)
    hop_ms = 5.0

    sp_list = []
    ap_list = []
    f0_list = []

    for y in segments:
        f0, t = pw.dio(y, sr, f0_floor=60.0, f0_ceil=800.0)
        f0 = pw.stonemask(y, f0, t, sr)
        sp = pw.cheaptrick(y, f0, t, sr)
        ap = pw.d4c(y, f0, t, sr)

        sp_list.append(sp)
        ap_list.append(ap)
        f0_list.append(f0)

    for i in range(n):
        target_frames = f0_list[i].shape[0]
        dur_ms = seg_dur_ms[i]
        if dur_ms > 0:
            dur_ms = dur_ms / args.speed
            target_frames = max(2, int(dur_ms / hop_ms))

        if target_frames != f0_list[i].shape[0]:
            sp_list[i] = resize_matrix(sp_list[i], target_frames)
            ap_list[i] = resize_matrix(ap_list[i], target_frames)
            f0_list[i] = resize_f0(f0_list[i], target_frames)

        pf = seg_pf[i]
        if pf != 1.0:
            voiced = f0_list[i] > 0
            f0_list[i] = np.where(voiced, f0_list[i] * pf, f0_list[i])

    total_frames = sum(f.shape[0] for f in f0_list)
    sp_dim = sp_list[0].shape[1]
    merged_sp = np.zeros((total_frames, sp_dim), dtype=np.float64)
    merged_ap = np.zeros((total_frames, sp_dim), dtype=np.float64)
    merged_f0 = np.zeros(total_frames, dtype=np.float64)

    cf_frames = max(2, int(args.crossfade_ms / hop_ms))
    offset = 0

    for i in range(n):
        nf = f0_list[i].shape[0]
        if i == 0:
            merged_sp[offset:offset + nf] = sp_list[i]
            merged_ap[offset:offset + nf] = ap_list[i]
            merged_f0[offset:offset + nf] = f0_list[i]
        else:
            cf = min(cf_frames, nf, total_frames - offset)
            cf = max(cf, 1)
            for j in range(nf):
                src = offset + j
                if src >= total_frames:
                    break
                if j < cf:
                    alpha = (j + 1) / (cf + 1)
                    merged_sp[src] = (1 - alpha) * merged_sp[src] + alpha * sp_list[i][j]
                    merged_ap[src] = (1 - alpha) * merged_ap[src] + alpha * ap_list[i][j]
                    merged_f0[src] = (1 - alpha) * merged_f0[src] + alpha * f0_list[i][j]
                else:
                    merged_sp[src] = sp_list[i][j]
                    merged_ap[src] = ap_list[i][j]
                    merged_f0[src] = f0_list[i][j]
        offset += nf

    merged_f0 = smooth_f0(merged_f0, window=5)

    y_out = pw.synthesize(merged_f0, merged_sp, merged_ap, sr)
    peak = np.abs(y_out).max()
    if peak > 0:
        y_out *= 0.95 / peak

    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    sf.write(args.out, y_out.astype(np.float32), sr)
    print(f"wrote {args.out} ({len(y_out)/sr:.1f}s, {sr}Hz)")


def load_plan(path: str) -> list[dict]:
    items = []
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                items.append(json.loads(line))
    return items


def resize_matrix(arr: np.ndarray, new_len: int) -> np.ndarray:
    if new_len <= 0:
        return arr[:1]
    old_len = arr.shape[0]
    idx = np.linspace(0, old_len - 1, new_len)
    idx_floor = np.floor(idx).astype(int)
    idx_ceil = np.minimum(idx_floor + 1, old_len - 1)
    frac = (idx - idx_floor)[:, np.newaxis]
    return arr[idx_floor] * (1 - frac) + arr[idx_ceil] * frac


def resize_f0(f0: np.ndarray, new_len: int) -> np.ndarray:
    if new_len <= 0:
        return f0[:1].copy()
    old_len = f0.shape[0]
    if new_len == old_len:
        return f0.copy()
    idx = np.linspace(0, old_len - 1, new_len)
    idx_floor = np.floor(idx).astype(int)
    idx_ceil = np.minimum(idx_floor + 1, old_len - 1)
    frac = idx - idx_floor
    result = np.zeros(new_len, dtype=np.float64)
    for i in range(new_len):
        result[i] = (1 - frac[i]) * f0[idx_floor[i]] + frac[i] * f0[idx_ceil[i]]
    return result


def smooth_f0(f0: np.ndarray, window: int = 5) -> np.ndarray:
    if window <= 1 or f0.shape[0] < window:
        return f0
    voiced = f0 > 0
    if not np.any(voiced):
        return f0
    smoothed = f0.copy()
    half = window // 2
    kernel = np.ones(window) / window
    smoothed_voiced = np.convolve(f0 * voiced.astype(np.float64), kernel, mode='same')
    denom = np.convolve(voiced.astype(np.float64), kernel, mode='same')
    valid = denom > 0
    smoothed[valid] = smoothed_voiced[valid] / denom[valid]
    smoothed[~voiced] = 0.0
    return smoothed


if __name__ == "__main__":
    main()
