import argparse
import subprocess
import sys
import os


def main() -> None:
    parser = argparse.ArgumentParser(description="Full UtauTTS pipeline: align -> train -> predict -> synth")
    parser.add_argument("command", choices=["align", "train", "synth", "full"], help="pipeline stage")
    parser.add_argument("--jsut-dir", default="data/jsut/basic5000", help="JSUT basic5000 directory")
    parser.add_argument("--aligned", default="data/jsut/aligned_v2.jsonl", help="aligned output path")
    parser.add_argument("--model", default="data/jsut/lstm_model.pth", help="model output/input path")
    parser.add_argument("--text", default="こんにちは、今日はいい天気ですね。", help="text to synthesize")
    parser.add_argument("--oto", default="sample/uta/oto.ini", help="oto.ini path")
    parser.add_argument("--out", default="out/synth.wav", help="output wav path")
    parser.add_argument("--epochs", type=int, default=50, help="training epochs")
    parser.add_argument("--batch", type=int, default=32, help="batch size")
    parser.add_argument("--lr", type=float, default=1e-3, help="learning rate")
    parser.add_argument("--vocoder", default="", help="HiFi-GAN checkpoint path for neural vocoder")
    parser.add_argument("--speed", type=float, default=1.0, help="speed factor")
    parser.add_argument("--f0-base", type=float, default=260.0, help="base F0 for fallback")
    args = parser.parse_args()

    tools = os.path.dirname(os.path.abspath(__file__))

    if args.command in ("align", "full"):
        print("=" * 60)
        print("Stage 1: JSUT alignment (acoustic boundary detection)")
        print("=" * 60)
        run([
            sys.executable,
            os.path.join(tools, "align_jsut.py"),
            "--jsut-dir", args.jsut_dir,
            "--out", args.aligned,
        ])

    if args.command in ("train", "full"):
        print("=" * 60)
        print("Stage 2: LSTM prosody model training")
        print("=" * 60)
        run([
            sys.executable,
            os.path.join(tools, "train_jsut.py"),
            "--dataset", args.aligned,
            "--out", args.model,
            "--epochs", str(args.epochs),
            "--batch", str(args.batch),
            "--lr", str(args.lr),
        ])

    if args.command in ("synth", "full"):
        plan_path = os.path.splitext(args.out)[0] + ".jsonl"

        print("=" * 60)
        print(f"Stage 3: DNN plan generation for text: {args.text}")
        print("=" * 60)
        run([
            sys.executable,
            os.path.join(tools, "predict_jsut.py"),
            "--text", args.text,
            "--oto", args.oto,
            "--model", args.model,
            "--out", plan_path,
        ])

        print("=" * 60)
        print("Stage 4: WORLD synthesis with prosody plan")
        print("=" * 60)
        synth_cmd = [
            sys.executable,
            os.path.join(tools, "prosody_synth.py"),
            "--oto", args.oto,
            "--plan", plan_path,
            "--out", args.out,
            "--speed", str(args.speed),
        ]
        if args.vocoder:
            synth_cmd.extend(["--vocoder", args.vocoder])
        run(synth_cmd)

    print(f"\ndone. output: {args.out}")


def run(cmd: list[str]) -> None:
    print(f"  {' '.join(cmd)}")
    result = subprocess.run(cmd)
    if result.returncode != 0:
        raise SystemExit(result.returncode)


if __name__ == "__main__":
    main()
