import argparse
import json
import math
import os
from dataclasses import dataclass

import numpy as np
import torch
from torch import nn
from torch.utils.data import DataLoader, Dataset


@dataclass
class Sample:
    features: np.ndarray
    targets: np.ndarray


class SimpleDataset(Dataset):
    def __init__(self, samples: list[Sample]):
        self.samples = samples

    def __len__(self) -> int:
        return len(self.samples)

    def __getitem__(self, idx: int):
        sample = self.samples[idx]
        return (
            torch.from_numpy(sample.features).float(),
            torch.from_numpy(sample.targets).float(),
        )


class MLP(nn.Module):
    def __init__(self, in_dim: int, out_dim: int):
        super().__init__()
        self.net = nn.Sequential(
            nn.Linear(in_dim, 64),
            nn.ReLU(),
            nn.Linear(64, 32),
            nn.ReLU(),
            nn.Linear(32, out_dim),
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return self.net(x)


def main() -> None:
    parser = argparse.ArgumentParser(description="Train a simple DNN for oto parameters")
    parser.add_argument("--dataset", required=True, help="path to dataset.jsonl")
    parser.add_argument("--out", required=True, help="output model path")
    parser.add_argument("--epochs", type=int, default=30, help="training epochs")
    parser.add_argument("--batch", type=int, default=64, help="batch size")
    parser.add_argument("--lr", type=float, default=1e-3, help="learning rate")
    parser.add_argument("--seed", type=int, default=42, help="random seed")
    args = parser.parse_args()

    torch.manual_seed(args.seed)
    np.random.seed(args.seed)

    samples = load_samples(args.dataset)
    if len(samples) < 10:
        raise ValueError("dataset too small")

    split = int(len(samples) * 0.9)
    train_samples = samples[:split]
    val_samples = samples[split:]

    train_loader = DataLoader(SimpleDataset(train_samples), batch_size=args.batch, shuffle=True)
    val_loader = DataLoader(SimpleDataset(val_samples), batch_size=args.batch, shuffle=False)

    model = MLP(in_dim=train_samples[0].features.shape[0], out_dim=train_samples[0].targets.shape[0])
    optimizer = torch.optim.Adam(model.parameters(), lr=args.lr)
    criterion = nn.MSELoss()

    best_val = math.inf
    best_state = None
    for epoch in range(1, args.epochs + 1):
        model.train()
        train_loss = 0.0
        for features, targets in train_loader:
            optimizer.zero_grad()
            pred = model(features)
            loss = criterion(pred, targets)
            loss.backward()
            optimizer.step()
            train_loss += loss.item() * features.size(0)
        train_loss /= len(train_samples)

        model.eval()
        val_loss = 0.0
        with torch.no_grad():
            for features, targets in val_loader:
                pred = model(features)
                loss = criterion(pred, targets)
                val_loss += loss.item() * features.size(0)
        val_loss /= max(1, len(val_samples))

        if val_loss < best_val:
            best_val = val_loss
            best_state = {k: v.cpu() for k, v in model.state_dict().items()}

        print(f"epoch={epoch} train={train_loss:.6f} val={val_loss:.6f}")

    out_dir = os.path.dirname(os.path.abspath(args.out))
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    if best_state is None:
        best_state = model.state_dict()
    torch.save(best_state, args.out)

    stats_path = os.path.splitext(args.out)[0] + ".json"
    stats = {
        "train_samples": len(train_samples),
        "val_samples": len(val_samples),
        "best_val": best_val,
        "features": FEATURE_NAMES,
        "targets": TARGET_NAMES,
    }
    with open(stats_path, "w", encoding="utf-8") as file:
        json.dump(stats, file, ensure_ascii=False, indent=2)


FEATURE_NAMES = [
    "frames",
    "duration_ms",
    "sample_rate",
    "f0_mean",
    "f0_std",
    "f0_median",
    "f0_q10",
    "f0_q90",
    "rms_mean",
    "rms_std",
    "rms_median",
    "rms_q10",
    "rms_q90",
    "f0_curve",
    "rms_curve",
]
TARGET_NAMES = ["preutterance_ms", "overlap_ms", "duration_ms", "rms_mean", "f0_curve", "rms_curve"]


def load_samples(path: str) -> list[Sample]:
    items: list[Sample] = []
    with open(path, "r", encoding="utf-8") as file:
        for line in file:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            f0 = np.array(record.get("f0", []), dtype=np.float32)
            rms = np.array(record.get("rms", []), dtype=np.float32)
            f0_curve = np.array(record.get("f0_curve", []), dtype=np.float32)
            rms_curve = np.array(record.get("rms_curve", []), dtype=np.float32)
            f0 = f0[np.isfinite(f0)]
            if f0.size == 0:
                f0_mean = f0_std = f0_median = f0_q10 = f0_q90 = 0.0
            else:
                f0_mean = float(np.mean(f0))
                f0_std = float(np.std(f0))
                f0_median = float(np.median(f0))
                f0_q10 = float(np.quantile(f0, 0.1))
                f0_q90 = float(np.quantile(f0, 0.9))
            if rms.size == 0:
                rms_mean = rms_std = rms_median = rms_q10 = rms_q90 = 0.0
            else:
                rms_mean = float(np.mean(rms))
                rms_std = float(np.std(rms))
                rms_median = float(np.median(rms))
                rms_q10 = float(np.quantile(rms, 0.1))
                rms_q90 = float(np.quantile(rms, 0.9))

            features = np.array(
                [
                    float(record.get("frames", 0)),
                    float(record.get("duration_ms", 0.0)),
                    float(record.get("sample_rate", 0)),
                    f0_mean,
                    f0_std,
                    f0_median,
                    f0_q10,
                    f0_q90,
                    rms_mean,
                    rms_std,
                    rms_median,
                    rms_q10,
                    rms_q90,
                ],
                dtype=np.float32,
            )
            features = np.concatenate([features, pad_curve(f0_curve), pad_curve(rms_curve)], axis=0)
            targets = np.array(
                [
                    float(record.get("preutterance_ms", 0.0)),
                    float(record.get("overlap_ms", 0.0)),
                    float(record.get("duration_ms", 0.0)),
                    float(record.get("rms_mean", 0.0)),
                ],
                dtype=np.float32,
            )
            targets = np.concatenate([targets, pad_curve(f0_curve), pad_curve(rms_curve)], axis=0)
            items.append(Sample(features=features, targets=targets))

    np.random.shuffle(items)
    return items


def pad_curve(values: np.ndarray, bins: int = 20) -> np.ndarray:
    if bins <= 0:
        return np.zeros(0, dtype=np.float32)
    if values.size == 0:
        return np.zeros(bins, dtype=np.float32)
    if values.size == bins:
        return values.astype(np.float32)
    if values.size > bins:
        return values[:bins].astype(np.float32)
    pad = np.zeros(bins - values.size, dtype=np.float32)
    return np.concatenate([values.astype(np.float32), pad], axis=0)


if __name__ == "__main__":
    main()
