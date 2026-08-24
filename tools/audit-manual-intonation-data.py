#!/usr/bin/env python3

import argparse
import json
import math
import statistics
import sys
from collections import Counter, defaultdict
from pathlib import Path


REQUIRED_ARRAYS = (
    "morae",
    "features",
    "base_points_cents",
    "mora_durations_ms",
    "mora_positions_ms",
    "manual_offsets_cents",
    "edit_mask",
)


def percentile(values, ratio):
    if not values:
        return 0.0
    ordered = sorted(values)
    index = max(0, math.ceil(len(ordered) * ratio) - 1)
    return ordered[index]


def numeric_list(value):
    return isinstance(value, list) and all(
        isinstance(item, (int, float)) and not isinstance(item, bool) and math.isfinite(item)
        for item in value
    )


def load_records(paths):
    records = []
    errors = []
    for path in paths:
        with path.open("r", encoding="utf-8-sig") as source:
            for line_number, line in enumerate(source, 1):
                if not line.strip():
                    continue
                try:
                    record = json.loads(line)
                except json.JSONDecodeError as error:
                    errors.append(f"{path}:{line_number}: JSON: {error}")
                    continue
                if not isinstance(record, dict):
                    errors.append(f"{path}:{line_number}: record must be an object")
                    continue
                record["_source"] = f"{path}:{line_number}"
                records.append(record)
    return records, errors


def validate_record(record):
    source = record["_source"]
    errors = []
    if record.get("version") != 1:
        errors.append(f"{source}: unsupported version")
    if not record.get("accepted") or record.get("source_kind") != "gui-confirmed":
        errors.append(f"{source}: record is not GUI-confirmed")
    if not str(record.get("text", "")).strip() or not str(record.get("reading", "")).strip():
        errors.append(f"{source}: text or reading is empty")
    morae = record.get("morae")
    if not isinstance(morae, list) or not morae:
        errors.append(f"{source}: morae is empty")
        return errors
    count = len(morae)
    for name in REQUIRED_ARRAYS:
        value = record.get(name)
        if not isinstance(value, list) or len(value) != count:
            errors.append(f"{source}: {name} length does not match morae")
    for name in ("base_points_cents", "mora_durations_ms", "mora_positions_ms", "manual_offsets_cents"):
        if name in record and not numeric_list(record[name]):
            errors.append(f"{source}: {name} contains a non-finite value")
    mask = record.get("edit_mask", [])
    if not all(isinstance(item, bool) for item in mask):
        errors.append(f"{source}: edit_mask contains a non-boolean value")
    if record.get("review_kind") not in ("edited-and-reviewed", "unchanged-and-reviewed"):
        errors.append(f"{source}: invalid review_kind")
    model = record.get("base_model", {})
    if model.get("id") != "frame-intonation-v8" or len(str(model.get("sha256", ""))) != 64:
        errors.append(f"{source}: invalid v8 identity")
    for index, mora in enumerate(morae):
        if isinstance(mora, dict) and mora.get("pause"):
            if index < len(mask) and mask[index]:
                errors.append(f"{source}: pause {index} is marked as edited")
            offsets = record.get("manual_offsets_cents", [])
            if index < len(offsets) and abs(offsets[index]) > 0.1:
                errors.append(f"{source}: pause {index} has a pitch offset")
    return errors


def feature_group(frame):
    for name, value in frame.items():
        if name.startswith("accent_type=") and value:
            return name.removeprefix("accent_type=")
    return "unknown"


def summarize(records, paths):
    prompt_ids = Counter()
    prompt_packs = Counter()
    session_ids = Counter()
    model_hashes = Counter()
    context_keys = Counter()
    edited_values = []
    all_values = []
    per_feature = defaultdict(list)
    per_record = []
    total_morae = pauses = editable = touched_zero = 0
    reviewed_unchanged = 0

    for record in records:
        prompt = record.get("prompt_set", {})
        prompt_id = str(prompt.get("prompt_id", ""))
        prompt_ids[prompt_id] += 1
        pack_id = str(prompt.get("pack_id", ""))
        if pack_id:
            prompt_packs[pack_id] += 1
        session_ids[str(record.get("session_id", ""))] += 1
        model_hashes[str(record.get("base_model", {}).get("sha256", ""))] += 1
        context = record.get("synthesis_context", {})
        context_keys[json.dumps(context, ensure_ascii=False, sort_keys=True)] += 1
        offsets = record["manual_offsets_cents"]
        mask = record["edit_mask"]
        morae = record["morae"]
        features = record["features"]
        record_edited = []
        total_morae += len(morae)
        for index, mora in enumerate(morae):
            if mora.get("pause"):
                pauses += 1
                continue
            editable += 1
            value = float(offsets[index])
            all_values.append(value)
            if mask[index]:
                edited_values.append(value)
                record_edited.append(value)
                per_feature[feature_group(features[index])].append(value)
                if abs(value) <= 0.5:
                    touched_zero += 1
            else:
                reviewed_unchanged += 1
        per_record.append(
            {
                "id": record.get("id", ""),
                "prompt_id": prompt_id,
                "pack_id": pack_id,
                "text": record.get("text", ""),
                "review_kind": record.get("review_kind", ""),
                "mora_count": len(morae),
                "edited_points": len(record_edited),
                "played_count": int(record.get("played_count", 0)),
                "mean_offset_cents": statistics.fmean(record_edited) if record_edited else 0.0,
                "max_abs_offset_cents": max(map(abs, record_edited), default=0.0),
            }
        )

    absolute = [abs(value) for value in edited_values]
    positive = sum(value > 0.5 for value in edited_values)
    negative = sum(value < -0.5 for value in edited_values)
    warnings = []
    if len(records) < 30:
        warnings.append("30発話未満のため、現時点ではschemaと操作性の確認用データとして扱う。")
    if edited_values and positive / len(edited_values) >= 0.7:
        warnings.append("上方向の補正が70%以上を占める。全体shiftと形状補正を分けて監査する。")
    over_limit = sum(value > 120 for value in absolute)
    if over_limit:
        warnings.append(f"±120 centを超える編集点が{over_limit}点ある。元値を保持し、学習targetだけを制限する。")
    duplicates = sorted(key for key, count in prompt_ids.items() if key and count > 1)
    if duplicates:
        warnings.append("重複prompt_id: " + ", ".join(duplicates))

    return {
        "format": "utautts-manual-prosody-audit",
        "format_version": 1,
        "inputs": [str(path) for path in paths],
        "records": len(records),
        "sessions": len(session_ids),
        "contexts": len(context_keys),
        "model_hashes": dict(model_hashes),
        "prompt_packs": dict(prompt_packs),
        "morae": {
            "total": total_morae,
            "pause": pauses,
            "editable": editable,
            "edited": len(edited_values),
            "reviewed_unchanged": reviewed_unchanged,
            "touched_zero": touched_zero,
            "edited_ratio": len(edited_values) / editable if editable else 0.0,
        },
        "offset_cents": {
            "positive": positive,
            "negative": negative,
            "zero": touched_zero,
            "mean": statistics.fmean(edited_values) if edited_values else 0.0,
            "median": statistics.median(edited_values) if edited_values else 0.0,
            "abs_median": statistics.median(absolute) if absolute else 0.0,
            "abs_p90": percentile(absolute, 0.90),
            "abs_p99": percentile(absolute, 0.99),
            "abs_max": max(absolute, default=0.0),
            "over_120": over_limit,
        },
        "accent_type": {
            key: {
                "count": len(values),
                "mean": statistics.fmean(values),
                "abs_mean": statistics.fmean(map(abs, values)),
            }
            for key, values in sorted(per_feature.items())
        },
        "per_record": per_record,
        "warnings": warnings,
    }


def main():
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description="UtauTTS manual prosody dataset audit")
    parser.add_argument("datasets", nargs="+", type=Path)
    parser.add_argument("--out", type=Path)
    args = parser.parse_args()

    records, errors = load_records(args.datasets)
    for record in records:
        errors.extend(validate_record(record))
    if errors:
        for error in errors:
            print(error)
        raise SystemExit(2)
    report = summarize(records, args.datasets)
    output = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    if args.out:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(output, encoding="utf-8")
    print(output, end="")


if __name__ == "__main__":
    main()
