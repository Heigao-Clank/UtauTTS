import argparse
import json
import math
import os
import re

import numpy as np
import pyopenjtalk
import torch
from torch import nn


class SequenceProsodyModel(nn.Module):
    def __init__(self, vocab_size: int, embed_dim: int = 64, hidden_dim: int = 128):
        super().__init__()
        self.embed = nn.Embedding(vocab_size + 1, embed_dim, padding_idx=0)
        lstm_in = embed_dim + 8
        self.lstm = nn.LSTM(lstm_in, hidden_dim, num_layers=2, batch_first=True, bidirectional=True, dropout=0.15)
        lstm_out = hidden_dim * 2
        self.dur_head = nn.Sequential(nn.Linear(lstm_out, 64), nn.ReLU(), nn.Dropout(0.1), nn.Linear(64, 1))
        self.f0_head = nn.Sequential(nn.Linear(lstm_out, 64), nn.ReLU(), nn.Dropout(0.1), nn.Linear(64, 1))
        self.rms_head = nn.Sequential(nn.Linear(lstm_out, 64), nn.ReLU(), nn.Dropout(0.1), nn.Linear(64, 1))

    def forward(self, kana_ids, scalar_feats, lengths):
        emb = self.embed(kana_ids)
        x = torch.cat([emb, scalar_feats], dim=-1)
        packed = nn.utils.rnn.pack_padded_sequence(x, lengths.cpu(), batch_first=True, enforce_sorted=False)
        lstm_out, _ = self.lstm(packed)
        lstm_out, _ = nn.utils.rnn.pad_packed_sequence(lstm_out, batch_first=True)
        return self.dur_head(lstm_out), self.f0_head(lstm_out), self.rms_head(lstm_out)


def main():
    parser = argparse.ArgumentParser(description="LSTM v3 DNN inference")
    parser.add_argument("--text", required=True)
    parser.add_argument("--oto", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--f0-base", type=float, default=0.0, help="base F0 Hz (0=auto)")
    args = parser.parse_args()

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

    data = torch.load(args.model, map_location=device)
    vocab_list = data["vocab"]
    vocab = {k: i for i, k in enumerate(vocab_list)}
    model = SequenceProsodyModel(vocab_size=len(vocab)).to(device)
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

    f0_base = args.f0_base
    if f0_base <= 0:
        f0_base = estimate_base_f0(entries, oto_dir)
        print(f"estimated voicebank f0_base: {f0_base:.0f} Hz")

    scalar = []
    for i in range(n):
        acc_val = kana_acc[i]
        mora_idx = kana_mora[i]
        is_last = 1.0 if i == n - 1 else 0.0
        is_accent_nucleus = 1.0 if (acc_val > 0 and mora_idx == acc_val - 1) else 0.0
        is_accent_top = 1.0 if (acc_val > 0 and mora_idx < acc_val - 1) else 0.0
        high_low = 1.0 if (acc_val == 0 or mora_idx < acc_val) else 0.0
        scalar.append([
            float(acc_val), float(mora_idx),
            float(i) / max(n - 1, 1), float(n), is_last,
            is_accent_nucleus, is_accent_top, high_low,
        ])

    k_ids = torch.tensor([[kana_idx(k) for k in kanas]], dtype=torch.long).to(device)
    scalar_t = torch.tensor([scalar], dtype=torch.float32).to(device)
    lengths = torch.tensor([n], dtype=torch.long).to(device)

    with torch.no_grad():
        dur_p, f0_p, rms_p = model(k_ids, scalar_t, lengths)
    dur_p = dur_p[0].cpu().numpy()
    f0_p = f0_p[0].cpu().numpy()
    rms_p = rms_p[0].cpu().numpy()

    records = []
    for i, k in enumerate(kanas):
        prev_k = kanas[i - 1] if i > 0 else ""
        entry = find_entry(entries, k, prev_k)
        if entry is None:
            continue

        dur_log = float(dur_p[i][0])
        dur_s = max(float(math.exp(dur_log)), 0.020)
        dur_s = min(dur_s, 0.600)
        dur_ms = dur_s * 1000.0

        f0_log = float(f0_p[i][0])
        f0_pred_hz = float(math.exp(f0_log))
        if is_unvoiced_kana(k):
            f0_pred_hz = 0.0
        else:
            f0_pred_hz = float(np.clip(f0_pred_hz, 120.0, 600.0))

        rms_log = float(rms_p[i][0])
        rms_scale = float(math.exp(rms_log))
        rms_scale = float(np.clip(rms_scale, 0.1, 5.0))

        records.append({
            "kana": k,
            "file": os.path.join(oto_dir, entry["file"]),
            "alias": entry["alias"],
            "offset_ms": entry["offset"],
            "blank_ms": entry["blank"],
            "target_dur_ms": round(dur_ms, 1),
            "f0_pred_hz": round(f0_pred_hz, 1),
            "rms_scale": round(rms_scale, 4),
        })

    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as f:
        for r in records:
            f.write(json.dumps(r, ensure_ascii=False))
            f.write("\n")
    print(f"written={len(records)}")


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


def is_unvoiced_kana(kana: str) -> bool:
    return kana in "っッ、。,.!? 　"


def estimate_base_f0(entries: list[dict], oto_dir: str) -> float:
    import pyworld as pw
    import soundfile as sf
    f0s = []
    for e in entries[:50]:
        wav_path = os.path.join(oto_dir, e["file"])
        if not os.path.exists(wav_path):
            continue
        try:
            y, sr = sf.read(wav_path, dtype="float64")
            if y.ndim > 1:
                y = np.mean(y, axis=1)
            f0, _ = pw.dio(y[:sr], sr, f0_floor=60.0, f0_ceil=800.0)
            f0_valid = f0[f0 > 0]
            if f0_valid.size > 0:
                f0s.append(float(np.median(f0_valid)))
        except Exception:
            pass
    return float(np.median(f0s)) if f0s else 260.0


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
