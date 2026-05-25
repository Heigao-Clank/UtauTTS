import argparse
import json
import math
import os
import re

import numpy as np
import pyopenjtalk
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


def main():
    parser = argparse.ArgumentParser(description="DNN duration prediction + accent-rule F0")
    parser.add_argument("--text", required=True)
    parser.add_argument("--oto", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--base-dur", type=float, default=100.0, help="base duration in ms (fallback)")
    parser.add_argument("--dur-scale", type=float, default=0.55, help="duration scale factor (tune for speed)")
    parser.add_argument("--f0-base", type=float, default=260.0, help="base F0 Hz for pitch rule")
    parser.add_argument("--f0-range", type=float, default=60.0, help="F0 variation range")
    args = parser.parse_args()

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

    data = torch.load(args.model, map_location=device)
    vocab_list = data["vocab"]
    vocab = {k: i for i, k in enumerate(vocab_list)}
    model = DurationModel(vocab_size=len(vocab)).to(device)
    model.load_state_dict(data["state"])
    model.eval()

    def kana_idx(kana: str) -> int:
        return vocab.get(kana, 0)

    kana_text = pyopenjtalk.g2p(args.text, kana=True)
    kanas = [kata_to_hira(k) for k in kana_text]
    if not kanas:
        raise ValueError("empty kana sequence")
    n = len(kanas)

    words = pyopenjtalk.run_frontend(args.text)
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

    oto_dir = os.path.dirname(os.path.abspath(args.oto))
    entries = load_oto(args.oto)

    # Build DNN input features
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

    # Generate F0 from accent rules (same logic as plan_synth.py)
    phrase_boundaries = []
    for i in range(n):
        if i == n - 1:
            phrase_boundaries.append(i + 1)
        elif i + 1 < n:
            w_cur = _word_idx_for_pos(i, w_info, kana_mora)
            w_next = _word_idx_for_pos(i + 1, w_info, kana_mora)
            if w_cur != w_next:
                phrase_boundaries.append(i + 1)

    records = []
    phrase_start = 0
    pi = 0
    for phrase_end in phrase_boundaries:
        if phrase_end >= n:
            phrase_end = n
        phrase_kanas = kanas[phrase_start:phrase_end]
        phrase_acc = kana_acc[phrase_start:phrase_end]
        phrase_mora = kana_mora[phrase_start:phrase_end]
        phrase_n = len(phrase_kanas)
        phrase_mora_count = sum(1 for m in phrase_mora if m == 0)

        for j in range(phrase_n):
            idx = phrase_start + j
            k = kanas[idx]
            prev_k = kanas[idx - 1] if idx > 0 else ""
            entry = find_entry(entries, k, prev_k)
            if entry is None:
                continue

            dur_log = float(dur_p[idx][0])
            dur_ms = max(float(math.exp(dur_log)) * 1000.0 * args.dur_scale, 30.0)
            dur_ms = min(dur_ms, 500.0)

            pf = compute_pitch_factor(
                phrase_acc[j], phrase_mora[j], phrase_mora_count,
                position_in_phrase=j, phrase_len=phrase_n,
                is_utterance_final=(idx == n - 1),
                is_utterance_initial=(idx == 0),
                f0_base=args.f0_base, f0_range=args.f0_range,
            )

            records.append({
                "kana": k,
                "file": os.path.join(oto_dir, entry["file"]),
                "alias": entry["alias"],
                "offset_ms": entry["offset"],
                "blank_ms": entry["blank"],
                "target_dur_ms": round(dur_ms, 1),
                "pitch_factor": round(pf, 3),
            })

        phrase_start = phrase_end if phrase_end < n else n
        pi += 1

    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as f:
        for r in records:
            f.write(json.dumps(r, ensure_ascii=False))
            f.write("\n")
    print(f"written={len(records)}")


def _word_idx_for_pos(pos: int, w_info: list[dict], kana_mora: list[int]) -> int:
    ki = 0
    for wi, wn in enumerate(w_info):
        for m in range(wn["mora_size"]):
            if ki == pos:
                return wi
            if ki < len(kana_mora) and kana_mora[ki] == 0 and m > 0:
                pass
            ki += 1
            if ki > pos:
                return wi
    return -1


def compute_pitch_factor(acc, mora_idx, mora_count, position_in_phrase, phrase_len, is_utterance_final, is_utterance_initial, f0_base=260.0, f0_range=60.0):
    pf = 1.0

    if acc == 0:
        if position_in_phrase < phrase_len * 0.3:
            pf = 0.92 + 0.08 * (position_in_phrase / max(phrase_len * 0.3, 1))
        else:
            pf = 1.0
    elif acc == 1:
        pf = 1.12 - 0.12 * (position_in_phrase / max(phrase_len - 1, 1))
    else:
        kernel_pos = acc - 1
        if mora_idx < kernel_pos:
            frac = mora_idx / max(kernel_pos, 1)
            pf = 0.88 + 0.24 * frac
        elif mora_idx == kernel_pos:
            pf = 1.12
        else:
            frac = (mora_idx - kernel_pos) / max(mora_count - kernel_pos, 1)
            pf = 1.12 - 0.30 * frac

    if is_utterance_initial and position_in_phrase < 0.15:
        pf -= 0.05

    if is_utterance_final:
        pf -= 0.05 * (position_in_phrase / max(phrase_len - 1, 1))

    if is_utterance_final and position_in_phrase == phrase_len - 1:
        pf -= 0.10

    return float(np.clip(pf, 0.6, 1.3))


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


def find_entry(entries: list[dict], kana: str, prev_kana: str = "") -> dict | None:
    if prev_kana and is_vowel(prev_kana):
        for e in entries:
            if e["alias"].strip() == f"- {kana}":
                return e
    elif prev_kana:
        for e in entries:
            if e["alias"].strip() == f"* {kana}":
                return e
    for e in entries:
        alias_clean = e["alias"].replace("- ", "").replace("* ", "").strip()
        if alias_clean == kana:
            return e
    for e in entries:
        alias_clean = e["alias"].replace("- ", "").replace("* ", "").strip()
        if alias_clean.endswith(kana) and len(alias_clean) <= len(kana) + 3:
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


if __name__ == "__main__":
    main()
