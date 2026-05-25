import argparse

import pyopenjtalk


def main() -> None:
    parser = argparse.ArgumentParser(description="Convert text to kana or phoneme")
    parser.add_argument("--text", required=True, help="input text")
    parser.add_argument(
        "--mode",
        default="kana",
        choices=["kana", "phoneme"],
        help="output mode",
    )
    args = parser.parse_args()

    if args.mode == "kana":
        print(pyopenjtalk.g2p(args.text, kana=True))
    else:
        print(pyopenjtalk.g2p(args.text, kana=False))


if __name__ == "__main__":
    main()
