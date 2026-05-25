import argparse
import json

import numpy as np
import torch


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
NUM_SCALAR_TARGET = 4


def main() -> None:
    parser = argparse.ArgumentParser(description="Run DNN inference on dataset")
    parser.add_argument("--dataset", required=True, help="path to dataset.jsonl")
    parser.add_argument("--model", required=True, help="path to model .pth")
    parser.add_argument("--out", required=True, help="output jsonl path")
    args = parser.parse_args()

    model = load_model(args.model, args.dataset)
    model.eval()

    with open(args.dataset, "r", encoding="utf-8") as dataset_file, open(
        args.out, "w", encoding="utf-8"
    ) as out_file:
        for line in dataset_file:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            features = extract_features(record)
            with torch.no_grad():
                pred = model(torch.from_numpy(features).float().unsqueeze(0))
            pred = pred.squeeze(0).cpu().numpy().tolist()
            output = {
                "alias": record.get("alias"),
                "source": record.get("source"),
                "trimmed": record.get("trimmed"),
                "targets": {
                    "preutterance_ms": record.get("preutterance_ms"),
                    "overlap_ms": record.get("overlap_ms"),
                    "duration_ms": record.get("duration_ms"),
                    "rms_mean": record.get("rms_mean"),
                    "f0_curve": record.get("f0_curve"),
                    "rms_curve": record.get("rms_curve"),
                },
                "predicted": {
                    "preutterance_ms": pred[0],
                    "overlap_ms": pred[1],
                    "duration_ms": pred[2],
                    "rms_mean": pred[3],
                    "f0_curve": pred[4 : 4 + CURVE_BINS],
                    "rms_curve": pred[4 + CURVE_BINS : 4 + CURVE_BINS * 2],
                },
            }
            out_file.write(json.dumps(output, ensure_ascii=False))
            out_file.write("\n")


class MLP(torch.nn.Module):
    def __init__(self, in_dim: int, out_dim: int):
        super().__init__()
        self.net = torch.nn.Sequential(
            torch.nn.Linear(in_dim, 64),
            torch.nn.ReLU(),
            torch.nn.Linear(64, 32),
            torch.nn.ReLU(),
            torch.nn.Linear(32, out_dim),
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return self.net(x)


def load_model(path: str, dataset_path: str) -> torch.nn.Module:
    first_sample = None
    with open(dataset_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                first_sample = json.loads(line)
                break
    if first_sample is None:
        raise ValueError("dataset is empty")
    feats = extract_features(first_sample)
    in_dim = feats.shape[0]
    out_dim = NUM_SCALAR_TARGET + 2 * CURVE_BINS
    model = MLP(in_dim=in_dim, out_dim=out_dim)
    state = torch.load(path, map_location="cpu")
    model.load_state_dict(state)
    return model


def extract_features(record: dict) -> np.ndarray:
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
    return features


CURVE_BINS = 20


def pad_curve(values: np.ndarray, bins: int = CURVE_BINS) -> np.ndarray:
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
