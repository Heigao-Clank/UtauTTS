import argparse
import json
import math
import os

import numpy as np
import torch
from torch import nn
from torch.nn import functional as F
from torch.utils.data import DataLoader, Dataset


class SequenceProsodyModel(nn.Module):
    def __init__(self, vocab_size: int, embed_dim: int = 64, hidden_dim: int = 128):
        super().__init__()
        self.embed = nn.Embedding(vocab_size + 1, embed_dim, padding_idx=0)
        lstm_in = embed_dim + 8
        self.lstm = nn.LSTM(lstm_in, hidden_dim, num_layers=2, batch_first=True, bidirectional=True, dropout=0.15)
        lstm_out = hidden_dim * 2

        self.dur_head = nn.Sequential(
            nn.Linear(lstm_out, 64), nn.ReLU(), nn.Dropout(0.1), nn.Linear(64, 1),
        )
        self.f0_head = nn.Sequential(
            nn.Linear(lstm_out, 64), nn.ReLU(), nn.Dropout(0.1), nn.Linear(64, 1),
        )
        self.rms_head = nn.Sequential(
            nn.Linear(lstm_out, 64), nn.ReLU(), nn.Dropout(0.1), nn.Linear(64, 1),
        )

    def forward(self, kana_ids, scalar_feats, lengths):
        emb = self.embed(kana_ids)
        x = torch.cat([emb, scalar_feats], dim=-1)
        packed = nn.utils.rnn.pack_padded_sequence(x, lengths.cpu(), batch_first=True, enforce_sorted=False)
        lstm_out, _ = self.lstm(packed)
        lstm_out, _ = nn.utils.rnn.pad_packed_sequence(lstm_out, batch_first=True)
        return self.dur_head(lstm_out), self.f0_head(lstm_out), self.rms_head(lstm_out)


class SequenceDataset(Dataset):
    def __init__(self, sequences: list[dict]):
        self.sequences = sequences

    def __len__(self):
        return len(self.sequences)

    def __getitem__(self, idx):
        seq = self.sequences[idx]
        n = len(seq["kana_ids"])
        return (
            torch.tensor(seq["kana_ids"], dtype=torch.long),
            torch.tensor(seq["scalar_feats"], dtype=torch.float32),
            torch.tensor(seq["dur_targets"], dtype=torch.float32),
            torch.tensor(seq["f0_targets"], dtype=torch.float32),
            torch.tensor(seq["rms_targets"], dtype=torch.float32),
            n,
        )


def collate_fn(batch):
    kana_ids, scalar, dur_tgt, f0_tgt, rms_tgt, lengths = zip(*batch)
    max_len = max(lengths)
    B = len(batch)
    pad_kana = torch.zeros(B, max_len, dtype=torch.long)
    pad_scalar = torch.zeros(B, max_len, 8, dtype=torch.float32)
    pad_dur = torch.zeros(B, max_len, 1, dtype=torch.float32)
    pad_f0 = torch.zeros(B, max_len, 1, dtype=torch.float32)
    pad_rms = torch.zeros(B, max_len, 1, dtype=torch.float32)
    for i in range(B):
        L = lengths[i]
        pad_kana[i, :L] = kana_ids[i]
        pad_scalar[i, :L] = scalar[i]
        pad_dur[i, :L] = dur_tgt[i]
        pad_f0[i, :L] = f0_tgt[i]
        pad_rms[i, :L] = rms_tgt[i]
    return pad_kana, pad_scalar, pad_dur, pad_f0, pad_rms, torch.tensor(lengths, dtype=torch.long)


def main():
    parser = argparse.ArgumentParser(description="Train LSTM prosody model v3")
    parser.add_argument("--dataset", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--epochs", type=int, default=80)
    parser.add_argument("--batch", type=int, default=64)
    parser.add_argument("--lr", type=float, default=1e-3)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--max-seq-len", type=int, default=200)
    args = parser.parse_args()

    torch.manual_seed(args.seed)
    np.random.seed(args.seed)
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    print(f"device: {device}")
    if device.type == "cuda":
        print(f"  gpu: {torch.cuda.get_device_name(0)} ({torch.cuda.get_device_properties(0).total_memory // 1024**3} GB)")

    records = load_records(args.dataset)
    if len(records) < 100:
        raise ValueError(f"dataset too small: {len(records)}")

    sequences, vocab = build_sequences(records, args.max_seq_len)
    print(f"vocab={len(vocab)} sequences={len(sequences)}")

    np.random.shuffle(sequences)
    split = int(len(sequences) * 0.9)
    train_ds = SequenceDataset(sequences[:split])
    val_ds = SequenceDataset(sequences[split:])

    train_loader = DataLoader(train_ds, batch_size=args.batch, shuffle=True, collate_fn=collate_fn)
    val_loader = DataLoader(val_ds, batch_size=args.batch, shuffle=False, collate_fn=collate_fn)

    model = SequenceProsodyModel(vocab_size=len(vocab)).to(device)
    print(f"params: {sum(p.numel() for p in model.parameters()):,}")

    optimizer = torch.optim.Adam(model.parameters(), lr=args.lr)
    scheduler = torch.optim.lr_scheduler.ReduceLROnPlateau(optimizer, patience=8, factor=0.5)
    l1 = nn.L1Loss()

    best_val = math.inf
    best_state = None
    for epoch in range(1, args.epochs + 1):
        model.train()
        train_loss = 0.0
        for batch in train_loader:
            k_ids, scalar, dur_tgt, f0_tgt, rms_tgt, lengths = batch
            k_ids = k_ids.to(device)
            scalar = scalar.to(device)
            dur_tgt = dur_tgt.to(device)
            f0_tgt = f0_tgt.to(device)
            rms_tgt = rms_tgt.to(device)
            lengths = lengths.to(device)

            optimizer.zero_grad()
            dur_p, f0_p, rms_p = model(k_ids, scalar, lengths)

            mask = torch.zeros(k_ids.shape[0], k_ids.shape[1], 1, device=device)
            for b in range(k_ids.shape[0]):
                mask[b, :lengths[b], 0] = 1.0

            dur_loss = l1(dur_p * mask, dur_tgt * mask)
            f0_loss = l1(f0_p * mask, f0_tgt * mask)
            rms_loss = l1(rms_p * mask, rms_tgt * mask)

            delta_loss = torch.tensor(0.0, device=device)
            for b in range(k_ids.shape[0]):
                L = lengths[b].item()
                if L >= 2:
                    f0_b = f0_p[b, :L, 0]
                    tgt_b = f0_tgt[b, :L, 0]
                    delta_loss += F.l1_loss(f0_b[1:] - f0_b[:-1], tgt_b[1:] - tgt_b[:-1])
            delta_loss /= max(1, k_ids.shape[0])

            loss = dur_loss + f0_loss + 0.3 * rms_loss + 0.5 * delta_loss
            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            optimizer.step()

            train_loss += loss.item() * lengths.sum().item()

        n_train = sum(len(s["kana_ids"]) for s in sequences[:split])
        train_loss /= max(1, n_train)

        model.eval()
        val_loss = 0.0
        with torch.no_grad():
            for batch in val_loader:
                k_ids, scalar, dur_tgt, f0_tgt, rms_tgt, lengths = batch
                k_ids = k_ids.to(device)
                scalar = scalar.to(device)
                dur_tgt = dur_tgt.to(device)
                f0_tgt = f0_tgt.to(device)
                rms_tgt = rms_tgt.to(device)
                lengths = lengths.to(device)
                dur_p, f0_p, rms_p = model(k_ids, scalar, lengths)
                mask = torch.zeros(k_ids.shape[0], k_ids.shape[1], 1, device=device)
                for b in range(k_ids.shape[0]):
                    mask[b, :lengths[b], 0] = 1.0
                dl = l1(dur_p * mask, dur_tgt * mask)
                fl = l1(f0_p * mask, f0_tgt * mask)
                rl = l1(rms_p * mask, rms_tgt * mask)
                deltal = torch.tensor(0.0, device=device)
                for b in range(k_ids.shape[0]):
                    L = lengths[b].item()
                    if L >= 2:
                        f0_b = f0_p[b, :L, 0]
                        tgt_b = f0_tgt[b, :L, 0]
                        deltal += F.l1_loss(f0_b[1:] - f0_b[:-1], tgt_b[1:] - tgt_b[:-1])
                deltal /= max(1, k_ids.shape[0])
                vl = dl + fl + 0.3 * rl + 0.5 * deltal
                val_loss += vl.item() * lengths.sum().item()

        n_val = sum(len(s["kana_ids"]) for s in sequences[split:])
        val_loss /= max(1, n_val)
        scheduler.step(val_loss)
        if val_loss < best_val:
            best_val = val_loss
            best_state = {k: v.cpu() for k, v in model.state_dict().items()}

        print(f"epoch={epoch} train={train_loss:.4f} val={val_loss:.4f}")

    out_dir = os.path.dirname(os.path.abspath(args.out))
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    if best_state is None:
        best_state = model.state_dict()
    torch.save({"state": best_state, "vocab": list(vocab.keys())}, args.out)
    print(f"saved {args.out}  best_val={best_val:.4f}")


def load_records(path: str) -> list[dict]:
    items = []
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                items.append(json.loads(line))
    return items


def build_sequences(records: list[dict], max_seq_len: int) -> tuple[list[dict], dict[str, int]]:
    groups: dict[str, list[dict]] = {}
    for r in records:
        uid = r.get("source", r.get("trimmed", ""))
        if not uid:
            continue
        groups.setdefault(uid, []).append(r)

    for uid in list(groups.keys()):
        groups[uid].sort(key=lambda x: x.get("position", 0))

    vocab: dict[str, int] = {"<pad>": 0}
    sequences = []

    for uid, recs in groups.items():
        if len(recs) < 2 or len(recs) > max_seq_len:
            continue

        kana_ids = []
        scalar_feats = []
        dur_targets = []
        f0_targets = []
        rms_targets = []
        prev_f0 = None

        for r in recs:
            kana = r.get("kana", "")
            if kana not in vocab:
                vocab[kana] = len(vocab)
            kana_ids.append(vocab[kana])

            acc_val = int(r.get("accent", 0))
            mora_idx = int(r.get("mora_idx", 0))
            mora_total = max(1, int(_mora_count_for_word(r)))
            pos = int(r.get("position", 0))
            total = max(1, int(r.get("total", 1)))
            is_last = 1.0 if pos == total - 1 else 0.0

            is_accent_nucleus = 1.0 if (acc_val > 0 and mora_idx == acc_val - 1) else 0.0
            is_accent_top = 1.0 if (acc_val > 0 and mora_idx < acc_val - 1) else 0.0
            high_low = 1.0 if (acc_val == 0 or mora_idx < acc_val) else 0.0

            scalar_feats.append([
                float(acc_val),
                float(mora_idx),
                float(pos) / max(total, 1),
                float(total),
                is_last,
                is_accent_nucleus,
                is_accent_top,
                high_low,
            ])

            dur_s = float(r.get("duration_ms", 0)) / 1000.0
            dur_log = math.log(max(dur_s, 0.005))
            dur_targets.append([dur_log])

            f0_hz = float(r.get("f0_mean", 0))
            if f0_hz > 0:
                f0_log = math.log(f0_hz)
                voiced_flag = 1.0
            else:
                f0_log = 0.0
                voiced_flag = 0.0
            f0_targets.append([f0_log])

            rms_val = float(r.get("rms_mean", 0))
            rms_log = math.log(max(rms_val, 1e-7))
            rms_targets.append([rms_log])

            prev_f0 = f0_hz if f0_hz > 0 else prev_f0

        sequences.append({
            "kana_ids": kana_ids,
            "scalar_feats": scalar_feats,
            "dur_targets": dur_targets,
            "f0_targets": f0_targets,
            "rms_targets": rms_targets,
        })

    return sequences, vocab


def _mora_count_for_word(rec: dict) -> int:
    return 1


if __name__ == "__main__":
    main()
