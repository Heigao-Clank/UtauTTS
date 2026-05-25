import argparse
import json
import math
import os

import numpy as np
import torch
from torch import nn
from torch.utils.data import DataLoader, Dataset


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
            n,
        )


def collate_fn(batch):
    kana_ids, scalar, dur_tgt, lengths = zip(*batch)
    max_len = max(lengths)
    B = len(batch)
    pad_kana = torch.zeros(B, max_len, dtype=torch.long)
    pad_scalar = torch.zeros(B, max_len, 8, dtype=torch.float32)
    pad_dur = torch.zeros(B, max_len, 1, dtype=torch.float32)
    for i in range(B):
        L = lengths[i]
        pad_kana[i, :L] = kana_ids[i]
        pad_scalar[i, :L] = scalar[i]
        pad_dur[i, :L] = dur_tgt[i]
    return pad_kana, pad_scalar, pad_dur, torch.tensor(lengths, dtype=torch.long)


def main():
    parser = argparse.ArgumentParser(description="Train duration-only LSTM model")
    parser.add_argument("--dataset", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--epochs", type=int, default=60)
    parser.add_argument("--batch", type=int, default=64)
    parser.add_argument("--lr", type=float, default=1e-3)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--max-seq-len", type=int, default=200)
    args = parser.parse_args()

    torch.manual_seed(args.seed)
    np.random.seed(args.seed)
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    print(f"device: {device}")

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

    model = DurationModel(vocab_size=len(vocab)).to(device)
    print(f"params: {sum(p.numel() for p in model.parameters()):,}")

    optimizer = torch.optim.Adam(model.parameters(), lr=args.lr)
    scheduler = torch.optim.lr_scheduler.ReduceLROnPlateau(optimizer, patience=8, factor=0.5)
    criterion = nn.L1Loss()

    best_val = math.inf
    best_state = None
    for epoch in range(1, args.epochs + 1):
        model.train()
        train_loss = 0.0
        for batch in train_loader:
            k_ids, scalar, dur_tgt, lengths = batch
            k_ids = k_ids.to(device)
            scalar = scalar.to(device)
            dur_tgt = dur_tgt.to(device)
            lengths = lengths.to(device)

            optimizer.zero_grad()
            dur_p = model(k_ids, scalar, lengths)

            mask = torch.zeros(k_ids.shape[0], k_ids.shape[1], 1, device=device)
            for b in range(k_ids.shape[0]):
                mask[b, :lengths[b], 0] = 1.0

            loss = criterion(dur_p * mask, dur_tgt * mask)
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
                k_ids, scalar, dur_tgt, lengths = batch
                k_ids = k_ids.to(device)
                scalar = scalar.to(device)
                dur_tgt = dur_tgt.to(device)
                lengths = lengths.to(device)
                dur_p = model(k_ids, scalar, lengths)
                mask = torch.zeros(k_ids.shape[0], k_ids.shape[1], 1, device=device)
                for b in range(k_ids.shape[0]):
                    mask[b, :lengths[b], 0] = 1.0
                val_loss += criterion(dur_p * mask, dur_tgt * mask).item() * lengths.sum().item()

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

        for r in recs:
            kana = r.get("kana", "")
            if kana not in vocab:
                vocab[kana] = len(vocab)
            kana_ids.append(vocab[kana])

            acc_val = int(r.get("accent", 0))
            mora_idx = int(r.get("mora_idx", 0))
            pos = int(r.get("position", 0))
            total = max(1, int(r.get("total", 1)))

            scalar_feats.append([
                float(acc_val),
                float(mora_idx),
                float(pos) / max(total, 1),
                float(total),
                1.0 if pos == total - 1 else 0.0,
                1.0 if (acc_val > 0 and mora_idx == acc_val - 1) else 0.0,
                1.0 if (acc_val > 0 and mora_idx < acc_val - 1) else 0.0,
                1.0 if (acc_val == 0 or mora_idx < acc_val) else 0.0,
            ])

            dur_s = max(float(r.get("duration_ms", 0)) / 1000.0, 0.005)
            dur_targets.append([math.log(dur_s)])

        sequences.append({
            "kana_ids": kana_ids,
            "scalar_feats": scalar_feats,
            "dur_targets": dur_targets,
        })

    return sequences, vocab


if __name__ == "__main__":
    main()
