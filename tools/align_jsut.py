import argparse
import json
import os
import re

import numpy as np
import pyopenjtalk
import pyworld as pw
import soundfile as sf

SIL_PHONEMES = {"xx", "sil", "pau", "cl"}
CONSONANT_UNVOICED = {"k", "s", "t", "h", "p", "f", "sh", "ch", "ts"}
CONSONANT_VOICED = {"g", "z", "d", "b", "v", "j", "n", "m", "r", "y", "w"}
VOWEL = {"a", "i", "u", "e", "o"}
NASAL = {"N"}

PHONEME_WEIGHT = {
    **{p: 0.22 for p in CONSONANT_UNVOICED},
    **{p: 0.30 for p in CONSONANT_VOICED},
    **{p: 1.10 for p in VOWEL},
    **{p: 0.80 for p in NASAL},
}

KANA_TO_PHONEMES: dict[str, list[str]] = {}


def main() -> None:
    parser = argparse.ArgumentParser(description="JSUT phoneme-level forced alignment via full-context labels")
    parser.add_argument("--jsut-dir", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--curve-bins", type=int, default=20)
    parser.add_argument("--fmin", type=float, default=60.0)
    parser.add_argument("--fmax", type=float, default=800.0)
    args = parser.parse_args()

    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)

    transcript = load_transcript(args.jsut_dir)
    wav_dir = find_wav_dir(args.jsut_dir)
    files = sorted(f for f in os.listdir(wav_dir) if f.lower().endswith(".wav"))

    total_kana = 0
    skipped = 0
    with open(args.out, "w", encoding="utf-8") as out:
        for wav_name in files:
            uid = os.path.splitext(wav_name)[0]
            text = transcript.get(uid, "")
            if not text:
                skipped += 1
                continue
            try:
                recs = process_one(os.path.join(wav_dir, wav_name), text, uid, args)
            except Exception as e:
                print(f"  skip {wav_name}: {e}")
                skipped += 1
                continue
            if not recs:
                skipped += 1
                continue
            for r in recs:
                out.write(json.dumps(r, ensure_ascii=False))
                out.write("\n")
                total_kana += 1
            if (total_kana + skipped) % 100 == 0:
                print(f"  progress: {total_kana + skipped} files...")
    print(f"written_kana={total_kana} skipped_files={skipped}")


def load_transcript(jsut_dir: str) -> dict[str, str]:
    for name in ["transcript_utf8.txt", "transcript.txt"]:
        tp = os.path.join(jsut_dir, name)
        if not os.path.exists(tp):
            continue
        result = {}
        with open(tp, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if ":" in line:
                    fid, text = line.split(":", 1)
                    result[fid.strip()] = text.strip()
        return result
    return {}


def find_wav_dir(jsut_dir: str) -> str:
    for sub in ["wav", "wave", "WAV"]:
        sp = os.path.join(jsut_dir, sub)
        if os.path.isdir(sp):
            return sp
    return jsut_dir


def process_one(wav_path: str, text: str, uid: str, args) -> list[dict] | None:
    try:
        full_labels = pyopenjtalk.extract_fullcontext(text)
    except Exception:
        return None

    phonemes = []
    for label in full_labels:
        p = _parse_phoneme(label)
        if p and p not in SIL_PHONEMES:
            phonemes.append(p)

    if len(phonemes) < 2:
        return None

    kana_text = pyopenjtalk.g2p(text, kana=True)
    kanas = [kata_to_hira(k) for k in kana_text]

    kana_to_phoneme_ranges = _map_phonemes_to_kana(phonemes, kanas)
    if not kana_to_phoneme_ranges:
        return None

    words = pyopenjtalk.run_frontend(text)
    word_info = [{"acc": w.get("acc", 0), "mora_size": w.get("mora_size", 1)} for w in words]

    kana_acc, kana_mora, kana_word = _distribute_accent(kanas, word_info)

    y, sr = sf.read(wav_path, dtype="float64")
    if y.ndim > 1:
        y = np.mean(y, axis=1)

    f0, t = pw.dio(y, sr, f0_floor=args.fmin, f0_ceil=args.fmax)
    f0 = pw.stonemask(y, f0, t, sr)
    sp = pw.cheaptrick(y, f0, t, sr)
    rms = np.sqrt(np.mean(sp ** 2, axis=1))

    total_frames = f0.shape[0]
    if total_frames < len(phonemes) * 2:
        return None

    boundaries = _phoneme_align(f0, rms, total_frames, phonemes)
    if len(boundaries) != len(phonemes) + 1:
        return None

    records = []
    for kana_idx, (k_start, k_end) in kana_to_phoneme_ranges.items():
        if k_start >= len(kanas) or k_end > len(kanas):
            continue
        kana = kanas[k_start]
        p_start = kana_to_phoneme_ranges.get(k_start, (0, 0))[0]
        p_end = kana_to_phoneme_ranges.get(k_end - 1, (0, 0))[1] if k_end > k_start else p_start + 1

        s = boundaries[p_start] if p_start < len(boundaries) else 0
        e = boundaries[p_end] if p_end < len(boundaries) else total_frames
        if e <= s:
            s = max(0, e - 2)
            e = s + 2

        seg_f0 = f0[s:e]
        seg_rms = rms[s:e]
        f0v = seg_f0[seg_f0 > 0]

        dur_ms = (e - s) * 5.0

        rec = {
            "source": uid,
            "kana": kana,
            "prev_kana": kanas[k_start - 1] if k_start > 0 else "",
            "next_kana": kanas[k_start + 1] if k_start + 1 < len(kanas) else "",
            "position": k_start,
            "total": len(kanas),
            "accent": kana_acc[k_start] if k_start < len(kana_acc) else 0,
            "mora_idx": kana_mora[k_start] if k_start < len(kana_mora) else 0,
            "word_idx": kana_word[k_start] if k_start < len(kana_word) else -1,
            "duration_ms": round(dur_ms, 1),
            "f0_mean": round(float(np.mean(f0v)), 2) if f0v.size else 0.0,
            "f0_std": round(float(np.std(f0v)), 2) if f0v.size else 0.0,
            "rms_mean": round(float(np.mean(seg_rms)), 6) if seg_rms.size else 0.0,
            "rms_std": round(float(np.std(seg_rms)), 6) if seg_rms.size else 0.0,
            "f0_curve": resize_curve(seg_f0, args.curve_bins).tolist(),
            "rms_curve": resize_curve(seg_rms, args.curve_bins).tolist(),
        }
        records.append(rec)

    return records


def _parse_phoneme(label: str) -> str:
    m = re.match(r"^\w+\^(\w+)-", label)
    if m:
        return m.group(1)
    return ""


def _map_phonemes_to_kana(phonemes: list[str], kanas: list[str]) -> dict[int, tuple[int, int]]:
    mapping: dict[int, tuple[int, int]] = {}
    ki = 0
    start_pi = 0
    for pi, p in enumerate(phonemes):
        is_mora_end = (p in VOWEL) or (p in NASAL)
        if is_mora_end:
            mapping[ki] = (start_pi, pi + 1)
            ki += 1
            start_pi = pi + 1
            if ki >= len(kanas):
                if pi + 1 < len(phonemes):
                    mapping[ki - 1] = (mapping[ki - 1][0], len(phonemes))
                break
    if ki < len(kanas) and start_pi < len(phonemes):
        mapping.setdefault(ki, (start_pi, len(phonemes)))

    if not mapping:
        step = max(1, len(phonemes) // len(kanas))
        for i in range(len(kanas)):
            s = i * step
            e = s + step if i < len(kanas) - 1 else len(phonemes)
            mapping[i] = (s, e)

    return mapping


def _distribute_accent(kanas: list[str], word_info: list[dict]) -> tuple[list[int], list[int], list[int]]:
    kana_acc, kana_mora, kana_word = [], [], []
    ki = 0
    for wi, wn in enumerate(word_info):
        for m in range(wn["mora_size"]):
            if ki < len(kanas):
                kana_acc.append(wn["acc"])
                kana_mora.append(m)
                kana_word.append(wi)
                ki += 1
    while len(kana_acc) < len(kanas):
        kana_acc.append(0)
        kana_mora.append(0)
        kana_word.append(-1)
    return kana_acc, kana_mora, kana_word


def _phoneme_align(f0: np.ndarray, rms: np.ndarray, total_frames: int, phonemes: list[str]) -> list[int]:
    n = len(phonemes)
    weights = np.array([PHONEME_WEIGHT.get(p, 0.80) for p in phonemes], dtype=np.float64)
    total_w = weights.sum()
    if total_w <= 0:
        return list(range(0, total_frames + 1, max(1, total_frames // n)))[:n + 1]

    target_frames = weights / total_w * total_frames

    noise_gate = np.median(rms) * 0.2
    voiced = (f0 > 60.0) & (rms > noise_gate)
    boundary_score = np.zeros(total_frames, dtype=np.float64)
    for t in range(1, total_frames):
        if voiced[t] != voiced[t - 1]:
            boundary_score[t] += 2.0
        dr = abs(float(rms[t]) - float(rms[t - 1]))
        denom = max(float(rms[t]), float(rms[t - 1]), 1e-9)
        boundary_score[t] += dr / denom * 1.5

    boundaries = [0]
    cum = 0.0
    for i in range(n - 1):
        cum += target_frames[i]
        ideal = int(round(cum))
        half_window = max(2, int(target_frames[i] * 0.35))
        lo = max(boundaries[-1] + 1, ideal - half_window)
        hi = min(total_frames - (n - i - 1), ideal + half_window)
        if hi <= lo:
            hi = lo + 1

        best_t = lo
        best_s = -1e9
        for t in range(lo, hi):
            score = boundary_score[t] - 0.02 * abs(t - ideal)
            if score > best_s:
                best_s = score
                best_t = t
        boundaries.append(best_t)

    boundaries.append(total_frames)
    return boundaries


def resize_curve(values: np.ndarray, bins: int) -> np.ndarray:
    if bins <= 0 or values.size == 0:
        return np.zeros(max(bins, 0), dtype=np.float32)
    x = np.linspace(0, values.size - 1, bins, dtype=np.float64)
    xi = np.floor(x).astype(int)
    xi1 = np.minimum(xi + 1, values.size - 1)
    frac = (x - xi).astype(np.float32)
    v = np.nan_to_num(values.astype(np.float32), nan=0.0)
    return (v[xi] * (1 - frac) + v[xi1] * frac).astype(np.float32)


def kata_to_hira(s: str) -> str:
    result = []
    for c in s:
        code = ord(c)
        if 0x30A0 <= code <= 0x30FF:
            result.append(chr(code - 0x60))
        else:
            result.append(c)
    return "".join(result)


if __name__ == "__main__":
    main()
