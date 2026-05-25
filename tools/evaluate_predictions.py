import argparse
import json
import math


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate prediction errors")
    parser.add_argument("--pred", required=True, help="path to predictions.jsonl")
    parser.add_argument("--out", default="", help="optional output json path")
    args = parser.parse_args()

    preutter_errors = []
    overlap_errors = []
    duration_errors = []
    rms_errors = []

    with open(args.pred, "r", encoding="utf-8") as file:
        for line in file:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            targets = record.get("targets", {})
            predicted = record.get("predicted", {})
            preutter_errors.append(
                to_float(predicted.get("preutterance_ms"))
                - to_float(targets.get("preutterance_ms"))
            )
            overlap_errors.append(
                to_float(predicted.get("overlap_ms"))
                - to_float(targets.get("overlap_ms"))
            )
            duration_errors.append(
                to_float(predicted.get("duration_ms"))
                - to_float(targets.get("duration_ms"))
            )
            rms_errors.append(
                to_float(predicted.get("rms_mean"))
                - to_float(targets.get("rms_mean"))
            )

    summary = {
        "count": len(preutter_errors),
        "preutter_mae": mae(preutter_errors),
        "preutter_rmse": rmse(preutter_errors),
        "overlap_mae": mae(overlap_errors),
        "overlap_rmse": rmse(overlap_errors),
        "duration_mae": mae(duration_errors),
        "duration_rmse": rmse(duration_errors),
        "rms_mae": mae(rms_errors),
        "rms_rmse": rmse(rms_errors),
    }

    if args.out:
        with open(args.out, "w", encoding="utf-8") as file:
            json.dump(summary, file, ensure_ascii=False, indent=2)
    else:
        print(json.dumps(summary, ensure_ascii=False, indent=2))


def mae(errors: list[float]) -> float:
    if not errors:
        return 0.0
    return sum(abs(x) for x in errors) / len(errors)


def rmse(errors: list[float]) -> float:
    if not errors:
        return 0.0
    return math.sqrt(sum(x * x for x in errors) / len(errors))


def to_float(value) -> float:
    if value is None:
        return 0.0
    return float(value)


if __name__ == "__main__":
    main()
