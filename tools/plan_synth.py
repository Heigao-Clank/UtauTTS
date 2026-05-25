import argparse
import json
import os
import re

import pyopenjtalk


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate synthesis plan from text + oto.ini")
    parser.add_argument("--text", required=True, help="input text")
    parser.add_argument("--oto", required=True, help="path to oto.ini")
    parser.add_argument("--out", required=True, help="output plan jsonl")
    parser.add_argument("--base-dur", type=float, default=120.0, help="base duration per kana (ms)")
    parser.add_argument("--f0-base", type=float, default=0.0, help="base F0 estimate (0=from samples)")
    args = parser.parse_args()

    oto_dir = os.path.dirname(os.path.abspath(args.oto))
    entries = load_oto(args.oto)

    kana_text = pyopenjtalk.g2p(args.text, kana=True)
    kanas = [kata_to_hira(k) for k in list(kana_text)]

    accent_info = get_word_info(args.text)
    total_moras = sum(w["mora_size"] for w in accent_info)

    records = []
    k_idx = 0
    for w_info in accent_info:
        mora_count = w_info["mora_size"]
        acc_pos = w_info["acc"]
        for m in range(mora_count):
            if k_idx >= len(kanas):
                break
            k = kanas[k_idx]
            prev_k = kanas[k_idx - 1] if k_idx > 0 else ""
            entry = find_entry(entries, k, prev_k, k_idx == 0)
            if entry is None:
                k_idx += 1
                continue

            # Duration: base ± 30% depending on position
            dur = args.base_dur
            if m == 0:
                dur *= 0.85
            elif m == mora_count - 1:
                dur *= 1.15

            # Pitch factor from accent
            pf = 1.0
            if acc_pos > 0:
                if m < acc_pos - 1:
                    pf = 0.95 + 0.05 * (m / max(acc_pos - 2, 1))
                elif m == acc_pos - 1:
                    pf = 1.05
                else:
                    pf = 1.05 - 0.1 * ((m - acc_pos + 1) / max(mora_count - acc_pos, 1))

            records.append({
                "kana": k,
                "file": os.path.join(oto_dir, entry["file"]),
                "alias": entry["alias"],
                "offset_ms": entry["offset"],
                "fixed_ms": entry["fixed"],
                "blank_ms": entry["blank"],
                "preutterance_ms": entry["preutterance"],
                "overlap_ms": entry["overlap"],
                "target_dur_ms": round(dur, 1),
                "pitch_factor": round(pf, 3),
            })
            k_idx += 1

    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as f:
        for rec in records:
            f.write(json.dumps(rec, ensure_ascii=False))
            f.write("\n")
    print(f"written={len(records)}")


def load_oto(oto_path: str) -> list[dict]:
    entries = []
    with open(oto_path, "r", encoding="shift_jis", errors="replace") as f:
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


def _is_kana_char(c: str) -> bool:
    o = ord(c)
    return (0x3040 <= o <= 0x309F) or (0x30A0 <= o <= 0x30FF)


def strip_alias(alias: str) -> str:
    alias = alias.strip()
    alias = alias.replace("- ", "").replace("* ", "")
    while alias and not _is_kana_char(alias[-1]):
        alias = alias[:-1]
    return alias.strip()


def _ctx_match(alias: str, prefix: str, kana: str) -> bool:
    a = alias.strip()
    if a.startswith(prefix):
        base = a[len(prefix):]
        base = base.strip()
        while base and not _is_kana_char(base[-1]):
            base = base[:-1]
        return base == kana
    return False


def find_entry(entries: list[dict], kana: str, prev_kana: str = "", is_first: bool = True) -> dict | None:
    if not is_first and prev_kana:
        if is_vowel(prev_kana):
            for e in entries:
                if _ctx_match(e["alias"], "- ", kana):
                    return e
        else:
            for e in entries:
                if _ctx_match(e["alias"], "* ", kana):
                    return e

    for e in entries:
        ac = strip_alias(e["alias"])
        if ac == kana:
            return e
    for e in entries:
        ac = strip_alias(e["alias"])
        if ac.startswith(kana):
            return e
    return None


def is_vowel(kana: str) -> bool:
    if not kana:
        return False
    if kana[-1] in "あいうえおぁぃぅぇぉアイウエオァィゥェォー":
        return True
    return False


def kata_to_hira(s: str) -> str:
    result = []
    for c in s:
        code = ord(c)
        if 0x30A0 <= code <= 0x30FF:
            result.append(chr(code - 0x60))
        else:
            result.append(c)
    return "".join(result)


def get_word_info(text: str) -> list[dict]:
    words = pyopenjtalk.run_frontend(text)
    return [{"mora_size": w.get("mora_size", 1), "acc": w.get("acc", 0)} for w in words]


if __name__ == "__main__":
    main()
