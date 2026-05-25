import argparse
import json
import math
import os
import re
import sys

import numpy as np
import pyopenjtalk
import pyworld as pw
import soundfile as sf
import torch
from torch import nn

class DurationModel(nn.Module):
    def __init__(self, vocab_size: int, embed_dim: int = 64, hidden_dim: int = 128):
        super().__init__()
        self.embed = nn.Embedding(vocab_size + 1, embed_dim, padding_idx=0)
        lstm_in = embed_dim + 8
        self.lstm = nn.LSTM(lstm_in, hidden_dim, num_layers=2, batch_first=True, bidirectional=True, dropout=0.15)
        self.head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 64), nn.ReLU(), nn.Dropout(0.1), nn.Linear(64, 1),
        )

    def forward(self, kana_ids, scalar_feats, lengths):
        emb = self.embed(kana_ids)
        x = torch.cat([emb, scalar_feats], dim=-1)
        packed = nn.utils.rnn.pack_padded_sequence(x, lengths.cpu(), batch_first=True, enforce_sorted=False)
        lstm_out, _ = self.lstm(packed)
        lstm_out, _ = nn.utils.rnn.pad_packed_sequence(lstm_out, batch_first=True)
        return self.head(lstm_out)

def load_oto(path: str) -> list[dict]:
    entries = []
    with open(path, "r", encoding="shift_jis", errors="replace") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            m = re.match(r"^(.+?)=(.+?),(\d+),(\d+),(-?\d+),(\d+),(\d+)", line)
            if not m:
                continue
            entries.append({
                "file": m.group(1).strip(),
                "alias": m.group(2).strip(),
                "offset": float(m.group(3)),
                "fixed": float(m.group(4)),
                "blank": float(m.group(5)),
                "preutterance": float(m.group(6)),
                "overlap": float(m.group(7)),
            })
    return entries

def _is_kana_char(c: str) -> bool:
    o = ord(c)
    return (0x3040 <= o <= 0x309F) or (0x30A0 <= o <= 0x30FF)


def strip_alias(alias: str) -> str:
    alias = alias.strip()
    alias = alias.replace("- ", "").replace("* ", "")
    while alias and not _is_kana_char(alias[-1]):
        alias = alias[:-1]
    return alias.strip()


def _ctx_match(alias: str, prefix: str, kana: str) -> bool:
    a = alias.strip()
    if a.startswith(prefix):
        base = a[len(prefix):]
        base = base.strip()
        while base and not _is_kana_char(base[-1]):
            base = base[:-1]
        return base == kana
    return False


def find_entry(entries: list[dict], kana: str, prev_kana: str = "") -> dict | None:
    if prev_kana and is_vowel(prev_kana):
        for e in entries:
            if _ctx_match(e["alias"], "- ", kana):
                return e
    elif prev_kana:
        for e in entries:
            if _ctx_match(e["alias"], "* ", kana):
                return e
    for e in entries:
        ac = strip_alias(e["alias"])
        if ac == kana:
            return e
    for e in entries:
        ac = strip_alias(e["alias"])
        if ac.endswith(kana) and len(ac) <= len(kana) + 3:
            return e
    return None


def is_vowel(kana: str) -> bool:
    return bool(kana) and kana[-1] in "あいうえおぁぃぅぇぉアイウエオァィゥェォー"


def kata_to_hira(s: str) -> str:
    result = []
    for c in s:
        code = ord(c)
        if 0x30A0 <= code <= 0x30FF:
            result.append(chr(code - 0x60))
        else:
            result.append(c)
    return "".join(result)

def compute_pitch_factors(kanas, kana_acc, kana_mora, w_info):
    n = len(kanas)

    phrase_starts = [0]
    for i in range(1, n):
        w_cur = _word_idx(i - 1, w_info, kana_mora)
        w_next = _word_idx(i, w_info, kana_mora)
        if w_cur != w_next:
            phrase_starts.append(i)

    result = [1.0] * n
    utterance_len = n

    for start, end in zip(phrase_starts, phrase_starts[1:] + [n]):
        phrase_len = end - start
        phrase_acc = kana_acc[start:end]
        phrase_mora = kana_mora[start:end]
        mora_count = 1
        for j in range(1, len(phrase_mora)):
            if phrase_mora[j] == 0:
                mora_count += 1

        for j, global_idx in enumerate(range(start, end)):
            acc = phrase_acc[j] if j < len(phrase_acc) else 0
            mora_idx = phrase_mora[j] if j < len(phrase_mora) else 0
            utt_pos = global_idx / max(utterance_len - 1, 1)

            pf = 1.0

            if acc == 0:
                phrase_progress = j / max(phrase_len - 1, 1)
                if phrase_progress < 0.25:
                    pf = 0.93 + 0.07 * (phrase_progress / 0.25)
            elif acc == 1:
                pf = 1.08 - 0.08 * (mora_idx / max(mora_count - 1, 1))
            elif acc >= 2:
                kernel = acc - 1
                if mora_idx < kernel:
                    frac = mora_idx / max(kernel, 1)
                    pf = 0.90 + 0.18 * frac
                elif mora_idx == kernel:
                    pf = 1.08
                else:
                    frac = (mora_idx - kernel) / max(mora_count - kernel, 1)
                    pf = 1.08 - 0.22 * frac

            declination = 1.0 - 0.06 * utt_pos
            pf *= declination

            if global_idx == utterance_len - 1:
                pf *= 0.94
            if global_idx == 0:
                pf *= 0.96
            if global_idx == start and global_idx > 0:
                pf *= 0.97

            result[global_idx] = round(float(np.clip(pf, 0.65, 1.25)), 3)

    return result


def _word_idx(pos, w_info, kana_mora):
    ki = 0
    for wi, wn in enumerate(w_info):
        for m in range(wn["mora_size"]):
            if ki == pos:
                return wi
            ki += 1
            if ki > pos:
                return wi
    return -1

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
    kernel = np.ones(window) / window
    smoothed_voiced = np.convolve(f0 * voiced.astype(np.float64), kernel, mode='same')
    denom = np.convolve(voiced.astype(np.float64), kernel, mode='same')
    valid = denom > 0
    smoothed[valid] = smoothed_voiced[valid] / denom[valid]
    smoothed[~voiced] = 0.0
    return smoothed

def synthesize(text: str, oto_path: str, model_path: str, out_path: str, dur_scale: float = 0.45, crossfade_ms: float = 20.0, speed: float = 1.0):
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

    data = torch.load(model_path, map_location=device)
    vocab_list = data["vocab"]
    vocab = {k: i for i, k in enumerate(vocab_list)}
    model = DurationModel(vocab_size=len(vocab)).to(device)
    model.load_state_dict(data["state"])
    model.eval()

    def kana_idx(kana: str) -> int:
        return vocab.get(kana, 0)

    kana_text = pyopenjtalk.g2p(text, kana=True)
    kanas = [kata_to_hira(k) for k in kana_text]
    if not kanas:
        raise ValueError("empty kana sequence")
    n = len(kanas)

    words = pyopenjtalk.run_frontend(text)
    w_info = [{"acc": w.get("acc", 0), "mora_size": w.get("mora_size", 1)} for w in words]

    kana_acc = []
    kana_mora = []
    ki = 0
    for wn in w_info:
        for m in range(wn["mora_size"]):
            if ki < n:
                kana_acc.append(wn["acc"])
                kana_mora.append(m)
                ki += 1
    while len(kana_acc) < n:
        kana_acc.append(0)
        kana_mora.append(0)

    oto_dir = os.path.dirname(os.path.abspath(oto_path))
    entries = load_oto(oto_path)

    scalar = []
    for i in range(n):
        acc_val = kana_acc[i]
        mora_idx = kana_mora[i]
        is_last = 1.0 if i == n - 1 else 0.0
        scalar.append([
            float(acc_val), float(mora_idx),
            float(i) / max(n - 1, 1), float(n), is_last,
            1.0 if (acc_val > 0 and mora_idx == acc_val - 1) else 0.0,
            1.0 if (acc_val > 0 and mora_idx < acc_val - 1) else 0.0,
            1.0 if (acc_val == 0 or mora_idx < acc_val) else 0.0,
        ])

    k_ids = torch.tensor([[kana_idx(k) for k in kanas]], dtype=torch.long).to(device)
    scalar_t = torch.tensor([scalar], dtype=torch.float32).to(device)
    lengths = torch.tensor([n], dtype=torch.long).to(device)

    with torch.no_grad():
        dur_p = model(k_ids, scalar_t, lengths)
    dur_p = dur_p[0].cpu().numpy()

    pitch_factors = compute_pitch_factors(kanas, kana_acc, kana_mora, w_info)

    segments = []
    seg_dur_ms = []
    seg_pf = []

    sr = None
    for i, k in enumerate(kanas):
        prev_k = kanas[i - 1] if i > 0 else ""
        entry = find_entry(entries, k, prev_k)
        if entry is None:
            continue

        wav_path = os.path.join(oto_dir, entry["file"])
        if not os.path.exists(wav_path):
            continue

        y, sr_file = sf.read(wav_path, dtype="float64")
        if y.ndim > 1:
            y = np.mean(y, axis=1)
        if sr is None:
            sr = sr_file
        if sr_file != sr:
            continue

        offset_ms = float(entry["offset"])
        blank_ms = float(entry["blank"])
        offset_s = max(0, int(offset_ms * sr / 1000))
        blank_s = int(blank_ms * sr / 1000)
        end = len(y) - blank_s
        if end <= offset_s:
            end = offset_s + int(100 * sr / 1000)
        if end > len(y):
            end = len(y)
        if end <= offset_s:
            end = offset_s + 1
        y = y[offset_s:end]

        dur_log = float(dur_p[i][0])
        dur_ms = max(float(math.exp(dur_log)) * 1000.0 * dur_scale, 30.0)
        dur_ms = min(dur_ms, 500.0)
        dur_ms *= 1.0 + np.random.normal(0, 0.04)

        segments.append(y)
        seg_dur_ms.append(dur_ms)
        seg_pf.append(pitch_factors[i])

    if not segments:
        raise ValueError("no valid segments")

    n_seg = len(segments)
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

    for i in range(n_seg):
        target_frames = f0_list[i].shape[0]
        dur_ms = seg_dur_ms[i]
        if dur_ms > 0:
            dur_ms = dur_ms / speed
            target_frames = max(2, int(dur_ms / hop_ms))

        if target_frames != f0_list[i].shape[0]:
            sp_list[i] = resize_matrix(sp_list[i], target_frames)
            ap_list[i] = resize_matrix(ap_list[i], target_frames)
            f0_list[i] = resize_f0(f0_list[i], target_frames)

        pf = seg_pf[i]
        if pf != 1.0:
            voiced = f0_list[i] > 0
            f0_list[i] = np.where(voiced, f0_list[i] * pf, f0_list[i])

    cf_frames = max(2, int(crossfade_ms / hop_ms))
    total_frames = sum(f.shape[0] for f in f0_list)
    sp_dim = sp_list[0].shape[1]
    merged_sp = np.zeros((total_frames, sp_dim), dtype=np.float64)
    merged_ap = np.zeros((total_frames, sp_dim), dtype=np.float64)
    merged_f0 = np.zeros(total_frames, dtype=np.float64)

    offset = 0
    for i in range(n_seg):
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

    voiced_mask = merged_f0 > 0
    if np.any(voiced_mask):
        jitter = np.random.normal(0, 1.5, merged_f0.shape).astype(np.float64)
        merged_f0 = np.where(voiced_mask, merged_f0 + jitter, merged_f0)
        merged_f0 = np.maximum(merged_f0, 0.0)

    sp_noise = np.random.normal(1.0, 0.008, merged_sp.shape).astype(np.float64)
    merged_sp *= sp_noise

    y_out = pw.synthesize(merged_f0, merged_sp, merged_ap, sr)
    peak = np.abs(y_out).max()
    if peak > 0:
        y_out *= 0.95 / peak

    os.makedirs(os.path.dirname(os.path.abspath(out_path)) or ".", exist_ok=True)
    sf.write(out_path, y_out.astype(np.float32), sr)
    duration = len(y_out) / sr
    print(f"wrote {out_path} ({duration:.1f}s, {sr}Hz)")
    return duration

def main():
    parser = argparse.ArgumentParser(description="UtauTTS synthesis engine")
    parser.add_argument("--text", required=True)
    parser.add_argument("--oto", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--dur-scale", type=float, default=0.45)
    parser.add_argument("--crossfade-ms", type=float, default=20.0)
    parser.add_argument("--speed", type=float, default=1.0)
    args = parser.parse_args()

    synthesize(
        text=args.text,
        oto_path=args.oto,
        model_path=args.model,
        out_path=args.out,
        dur_scale=args.dur_scale,
        crossfade_ms=args.crossfade_ms,
        speed=args.speed,
    )


if __name__ == "__main__":
    main()
