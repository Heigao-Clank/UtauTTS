import argparse
import json
import os


def main() -> None:
    parser = argparse.ArgumentParser(description="Prepare dataset for DNN training")
    parser.add_argument("--manifest", required=True, help="path to manifest.jsonl")
    parser.add_argument("--features", required=True, help="path to features.jsonl")
    parser.add_argument("--out", required=True, help="output jsonl path")
    args = parser.parse_args()

    manifest = load_jsonl(args.manifest)
    features = load_jsonl(args.features)

    feature_map = {item["trimmed"]: item for item in features}
    out_dir = os.path.dirname(os.path.abspath(args.out))
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)

    with open(args.out, "w", encoding="utf-8") as out_file:
        for entry in manifest:
            trimmed = entry.get("trimmed")
            if not trimmed:
                continue
            feat = feature_map.get(trimmed)
            if feat is None:
                continue
            frames = float(entry.get("frames", 0) or 0)
            sample_rate = float(entry.get("sample_rate", 0) or 0)
            duration_ms = 0.0
            if sample_rate > 0:
                duration_ms = (frames / sample_rate) * 1000.0

            record = {
                "alias": entry.get("alias"),
                "source": entry.get("source"),
                "trimmed": trimmed,
                "sample_rate": entry.get("sample_rate"),
                "channels": entry.get("channels"),
                "frames": entry.get("frames"),
                "duration_ms": duration_ms,
                "offset_ms": entry.get("offset_ms"),
                "fixed_ms": entry.get("fixed_ms"),
                "blank_ms": entry.get("blank_ms"),
                "preutterance_ms": entry.get("preutterance_ms"),
                "overlap_ms": entry.get("overlap_ms"),
                "hop_length": feat.get("hop_length"),
                "f0": feat.get("f0"),
                "rms": feat.get("rms"),
                "f0_curve": feat.get("f0_curve"),
                "rms_curve": feat.get("rms_curve"),
            }
            out_file.write(json.dumps(record, ensure_ascii=False))
            out_file.write("\n")


def load_jsonl(path: str) -> list[dict]:
    items: list[dict] = []
    with open(path, "r", encoding="utf-8") as file:
        for line in file:
            line = line.strip()
            if not line:
                continue
            items.append(json.loads(line))
    return items


if __name__ == "__main__":
    main()
