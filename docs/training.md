# Prosody training pipeline

This pipeline learns timing, relative pitch and relative energy from natural
speech. It does **not** learn the corpus speaker's waveform or absolute pitch;
the UTAU voicebank remains the sound source.

## 1. Build an auditable dataset

Place JSUT `basic5000` at `data/jsut/basic5000`, including
`transcript_utf8.txt` and `wav/*.wav`, then run:

```powershell
go run ./cmd/prosody-dataset --jsut data/jsut/basic5000 --out out/prosody/jsut.jsonl
```

Each JSONL row contains the text, Kagome reading, speech range, mora boundaries,
duration, F0, energy, and utterance-normalized ratios. Output order is stable
even though extraction runs in parallel. Rejected utterances and their reasons
are written to `<out>.report.json`.

JSUT does not include phoneme timings. The current extractor therefore uses a
weak monotonic alignment based on expected mora timing, local energy minima and
waveform correlation. These boundaries are inspectable intermediate data, not
hidden training state. A later forced aligner can replace extraction without
changing the model or renderer interfaces.

Use `--limit 100` for a quick experiment and `--workers N` to control CPU use.

## 2. Train and evaluate

```powershell
go run ./cmd/prosody-train --dataset out/prosody/jsut.jsonl --out out/prosody/model.json --epochs 30
```

The deterministic trainer holds out whole utterances by a stable hash of their
IDs. It fits sparse contextual models to log duration, log F0 ratio and log
energy ratio using Adam with a Huber-style clipped gradient. The model JSON
contains the split's duration MAE (ms), pitch MAE (cents), and energy-ratio MAE.
Because the split is by utterance, adjacent morae from one recording cannot
leak into both training and validation.

## 3. Synthesize with the model

```powershell
go run ./cmd/utautts --voicebank sample/uta --text "こんにちは。" --prosody out/prosody/model.json --out out.wav --plan-out plan.json
```

For the browser/API server:

```powershell
go run ./cmd/utautts-server --voice-dir sample --prosody out/prosody/model.json
```

Omitting `--prosody` keeps the fixed deterministic baseline. A plan generated
with a model records every predicted duration, pitch factor, and energy factor.

## Known limitations

- Weak boundaries are useful for bootstrapping, but are not accurate phoneme
  labels. Validation loss measures agreement with those extracted labels.
- The compact contextual regressor is deliberately a baseline. It does not yet
  model Japanese accent phrases or long-range sentence context.
- Pitch shifting is deterministic WSOLA plus resampling, not a vocoder. Large
  shifts are clamped to protect voice identity and audio quality.
- Naturalness must ultimately be checked with fixed listening tests, alongside
  objective prosody metrics and boundary discontinuity measurements.
