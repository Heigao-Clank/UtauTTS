# Roadmap

## Milestone 1: voicebank model

- Recursively load and merge `oto.ini` files.
- Preserve every candidate for duplicate aliases.
- Detect metadata encoding and report malformed entries.
- Classify alias patterns as CV, VCV and VC.
- Produce a coverage report for a representative Japanese sentence set.

## Milestone 2: deterministic renderer

- Define a synthesis-plan format independent of WAV processing.
- Resolve phonetic context without silently dropping morae.
- Implement `preutterance`, `overlap`, consonant and cutoff timing correctly.
- Add pitch-synchronous duration and F0 transformation.
- Verify placement and crossfades with generated test signals.

## Milestone 3: corpus prosody

- [x] Extract auditable weak mora alignment from JSUT speech and transcripts.
- [x] Train duration, relative F0, relative energy and pause baselines.
- [x] Keep speaker normalization separate from voicebank rendering.
- [x] Report held-out duration, pitch and energy metrics.
- [ ] Replace weak timing with a forced aligner and add fixed listening samples.

## Milestone 4: learned joins

- Build paired clean/degraded boundary examples from natural speech.
- Predict a short spectral residual or blending mask around each boundary.
- Condition on both neighboring units and the target prosody.
- Compare against the deterministic renderer before accepting the model.
