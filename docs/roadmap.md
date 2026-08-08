# Roadmap

## Milestone 1: voicebank model

- [x] Recursively load and merge `oto.ini` files.
- [x] Preserve every candidate for duplicate aliases.
- [x] Detect metadata encoding and report malformed entries.
- [x] Classify alias patterns as CV, VCV and VC.
- [ ] Produce a coverage report for a representative Japanese sentence set.

## Milestone 2: deterministic renderer

- [x] Define a synthesis-plan format independent of WAV processing.
- [x] Resolve phonetic context without silently dropping morae.
- [x] Implement `preutterance`, `overlap`, consonant and cutoff timing.
- [ ] Add pitch-synchronous F0 transformation.
- [x] Verify placement and crossfades with generated test signals.
- [x] Normalize pathological long-VCV timing without changing ordinary banks.
- [x] Record original and effective timing values in the synthesis plan.

## Milestone 3: corpus prosody

- [x] Extract auditable weak mora alignment from JSUT speech and transcripts.
- [x] Train conservative speech-duration residuals around the fixed baseline.
- [x] Keep speaker normalization separate from voicebank rendering.
- [x] Report held-out, speaking-rate-normalized duration metrics.
- [ ] Reintroduce pause, F0 and energy only after their targets are reliable.
- [ ] Replace weak timing with a forced aligner and add fixed listening samples.

## Milestone 4: learned joins

- Build paired clean/degraded boundary examples from natural speech.
- Predict a short spectral residual or blending mask around each boundary.
- Condition on both neighboring units and the target prosody.
- Compare against the deterministic renderer before accepting the model.
