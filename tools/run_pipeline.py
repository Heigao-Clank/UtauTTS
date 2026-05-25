import argparse
import subprocess
import sys


def main() -> None:
    parser = argparse.ArgumentParser(description="Run feature -> train -> predict -> evaluate pipeline")
    parser.add_argument("--manifest", required=True, help="path to manifest.jsonl")
    parser.add_argument("--workdir", required=True, help="output directory (export/model)")
    parser.add_argument("--model", default="simple.pth", help="model filename")
    parser.add_argument("--epochs", type=int, default=30, help="training epochs")
    parser.add_argument("--batch", type=int, default=64, help="batch size")
    parser.add_argument("--lr", type=float, default=1e-3, help="learning rate")
    args = parser.parse_args()

    workdir = args.workdir
    features = f"{workdir}/features.jsonl"
    dataset = f"{workdir}/dataset.jsonl"
    predictions = f"{workdir}/predictions.jsonl"
    model = f"{workdir}/models/{args.model}"

    run([
        sys.executable,
        "tools/extract_features.py",
        "--manifest",
        args.manifest,
        "--out",
        features,
    ])
    run([
        sys.executable,
        "tools/prepare_dataset.py",
        "--manifest",
        args.manifest,
        "--features",
        features,
        "--out",
        dataset,
    ])
    run([
        sys.executable,
        "tools/train_dnn.py",
        "--dataset",
        dataset,
        "--out",
        model,
        "--epochs",
        str(args.epochs),
        "--batch",
        str(args.batch),
        "--lr",
        str(args.lr),
    ])
    run([
        sys.executable,
        "tools/predict_dnn.py",
        "--dataset",
        dataset,
        "--model",
        model,
        "--out",
        predictions,
    ])
    run([
        sys.executable,
        "tools/evaluate_predictions.py",
        "--pred",
        predictions,
    ])


def run(cmd: list[str]) -> None:
    result = subprocess.run(cmd)
    if result.returncode != 0:
        raise SystemExit(result.returncode)


if __name__ == "__main__":
    main()
