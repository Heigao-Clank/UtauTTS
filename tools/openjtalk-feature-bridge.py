#!/usr/bin/env python3
"""Open JTalk frontend bridge for the portable UtauTTS runtime.

The executable built from this file reads {"text": "..."} from stdin and
writes the reading plus v8-compatible sparse mora feature frames as JSON.
"""

import argparse
import json
import sys

import openjtalk

from openjtalk_feature_common import analyze, sparse_features


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dictionary", required=True)
    args = parser.parse_args()
    request = json.load(sys.stdin)
    text = str(request.get("text", "")).strip()
    if not text:
        raise ValueError("text is empty")
    frontend = openjtalk.OpenJTalk(dn_mecab=args.dictionary.encode("utf-8"))
    reading, tokens = analyze(frontend, text)
    response = {
        "version": 1,
        "reading": reading,
        "morae": [token.get("mora", "") for token in tokens],
        "features": [sparse_features(token) for token in tokens],
    }
    json.dump(response, sys.stdout, ensure_ascii=False, separators=(",", ":"))
    sys.stdout.write("\n")


if __name__ == "__main__":
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
    main()
