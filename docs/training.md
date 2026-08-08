# Prosody training pipeline

This pipeline learns only local speech-duration corrections around the fixed
renderer baseline. It does **not** transfer the corpus speaker's waveform,
global speaking rate, pitch, energy, or pauses; the UTAU voicebank and baseline
renderer remain the sound source and timing anchor.

## 1. Build an auditable dataset

Place JSUT `basic5000` at `data/jsut/basic5000`, including
`transcript_utf8.txt` and `wav/*.wav`, then run:

```powershell
go run ./cmd/prosody-dataset --jsut data/jsut/basic5000 --out out/prosody/jsut.jsonl
```

Each JSONL row contains the text, Kagome reading, speech range, mora boundaries,
duration, F0, and energy. Output order is stable
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
IDs. It divides each speech duration by the fixed mora baseline, removes the
utterance's median speaking rate, and fits a sparse contextual model to the
remaining log-duration residual using Adam with a Huber-style clipped gradient.
The model JSON contains speaking-rate-normalized duration MAE. Because the
split is by utterance, adjacent morae from one recording cannot leak into both
training and validation.

## 3. Synthesize with the model

```powershell
go run ./cmd/utautts --voicebank sample/uta --text "こんにちは。" --prosody out/prosody/model.json --out out.wav --plan-out plan.json
```

For the browser/API server:

```powershell
go run ./cmd/utautts-server --voice-dir sample --prosody out/prosody/model.json
```

Omitting `--prosody` keeps the fixed deterministic baseline. With a model,
speech factors are centered to a median of 1.0 and clamped to 0.8--1.25. Pitch
and energy factors remain 1.0, and pauses remain at the configured baseline.

## Known limitations

- Weak boundaries are useful for bootstrapping, but are not accurate phoneme
  labels. Validation loss measures agreement with those extracted labels.
- The compact contextual regressor is deliberately a baseline. It does not yet
  model Japanese accent phrases or long-range sentence context.
- F0, energy, and pause learning are intentionally disabled until stronger
  alignment and accent targets are available.
- Naturalness must ultimately be checked with fixed listening tests, alongside
  objective prosody metrics and boundary discontinuity measurements.
