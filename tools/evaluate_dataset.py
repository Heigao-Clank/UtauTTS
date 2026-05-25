import argparse
import json
import statistics

import numpy as np


def main() -> None:
    parser = argparse.ArgumentParser(description="Summarize dataset features")
    parser.add_argument("--dataset", required=True, help="path to dataset.jsonl")
    parser.add_argument("--out", default="", help="optional output json path")
    args = parser.parse_args()

    f0_means = []
    rms_means = []
    preutterances = []
    overlaps = []

    with open(args.dataset, "r", encoding="utf-8") as file:
        for line in file:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            f0 = np.array(record.get("f0", []), dtype=np.float32)
            f0 = f0[np.isfinite(f0)]
            if f0.size:
                f0_means.append(float(np.mean(f0)))
            rms = np.array(record.get("rms", []), dtype=np.float32)
            if rms.size:
                rms_means.append(float(np.mean(rms)))
            preutterances.append(float(record.get("preutterance_ms", 0.0)))
            overlaps.append(float(record.get("overlap_ms", 0.0)))

    summary = {
        "count": len(preutterances),
        "f0_mean": safe_mean(f0_means),
        "f0_median": safe_median(f0_means),
        "rms_mean": safe_mean(rms_means),
        "rms_median": safe_median(rms_means),
        "preutterance_mean": safe_mean(preutterances),
        "preutterance_median": safe_median(preutterances),
        "overlap_mean": safe_mean(overlaps),
        "overlap_median": safe_median(overlaps),
    }

    if args.out:
        with open(args.out, "w", encoding="utf-8") as file:
            json.dump(summary, file, ensure_ascii=False, indent=2)
    else:
        print(json.dumps(summary, ensure_ascii=False, indent=2))


def safe_mean(values: list[float]) -> float:
    if not values:
        return 0.0
    return float(statistics.mean(values))


def safe_median(values: list[float]) -> float:
    if not values:
        return 0.0
    return float(statistics.median(values))


if __name__ == "__main__":
    main()
