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
    parser.add_argument("--dur-scale", type=float, default=0.45, help="duration scale factor (lower=faster)")
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

    pitch_factors = compute_pitch_factors(kanas, kana_acc, kana_mora, w_info, args.f0_base, args.f0_range)

    records = []
    for i, k in enumerate(kanas):
        prev_k = kanas[i - 1] if i > 0 else ""
        entry = find_entry(entries, k, prev_k)
        if entry is None:
            continue

        dur_log = float(dur_p[i][0])
        dur_ms = max(float(math.exp(dur_log)) * 1000.0 * args.dur_scale, 30.0)
        dur_ms = min(dur_ms, 500.0)
        dur_ms *= 1.0 + np.random.normal(0, 0.04)
        pf = pitch_factors[i]

        records.append({
            "kana": k,
            "file": os.path.join(oto_dir, entry["file"]),
            "alias": entry["alias"],
            "offset_ms": entry["offset"],
            "fixed_ms": entry["fixed"],
            "blank_ms": entry["blank"],
            "target_dur_ms": round(dur_ms, 1),
            "pitch_factor": round(pf, 3),
        })

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


def compute_pitch_factors(kanas, kana_acc, kana_mora, w_info, f0_base=260.0, f0_range=70.0):
    n = len(kanas)

    phrase_starts = [0]
    for i in range(1, n):
        w_cur = _word_idx_for_pos(i - 1, w_info, kana_mora)
        w_next = _word_idx_for_pos(i, w_info, kana_mora)
        if w_cur != w_next:
            phrase_starts.append(i)

    total_mora_positions = []
    for start, end in zip(phrase_starts, phrase_starts[1:] + [n]):
        phrase_kanas = kanas[start:end]
        phrase_acc = kana_acc[start:end]
        phrase_mora = kana_mora[start:end]

        mora_count = 1
        for j in range(1, len(phrase_mora)):
            if phrase_mora[j] == 0:
                mora_count += 1

        for j in range(len(phrase_kanas)):
            global_idx = start + j
            acc = phrase_acc[j] if j < len(phrase_acc) else 0
            mora_idx = phrase_mora[j] if j < len(phrase_mora) else 0
            total_mora_positions.append({
                'idx': global_idx,
                'acc': acc,
                'mora_idx': mora_idx,
                'mora_count': mora_count,
                'phrase_start': start,
                'phrase_end': end,
                'phrase_len': end - start,
                'is_first': global_idx == 0,
                'is_last': global_idx == n - 1,
            })

    result = [1.0] * n
    utterance_len = n

    for tp in total_mora_positions:
        i = tp['idx']
        acc = tp['acc']
        mora_idx = tp['mora_idx']
        mora_count = tp['mora_count']
        phrase_len = tp['phrase_len']

        utt_pos = i / max(utterance_len - 1, 1)

        pf = 1.0

        if acc == 0:
            phrase_progress = (i - tp['phrase_start']) / max(phrase_len - 1, 1)
            if phrase_progress < 0.25:
                pf = 0.93 + 0.07 * (phrase_progress / 0.25)
            else:
                pf = 1.0
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

        if tp['is_last']:
            pf *= 0.94

        if tp['is_first']:
            pf *= 0.96

        if i == tp['phrase_start'] and i > 0:
            pf *= 0.97

        result[i] = round(float(np.clip(pf, 0.65, 1.25)), 3)

    return result


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


if __name__ == "__main__":
    main()
