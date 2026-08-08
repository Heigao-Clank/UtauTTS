# UtauTTS architecture

UtauTTS aims to synthesize Japanese speech from an existing UTAU voicebank
without training a new acoustic model for that voicebank.

The system is split into four layers:

1. **Frontend** uses the embedded Kagome IPA dictionary to convert Japanese
   text into a pronunciation, then splits it into morae and pauses. Accent
   phrases remain a future layer.
2. **Prosody** predicts duration and utterance-normalized F0/energy ratios from
   natural speech corpora. The training dataset and held-out metrics are
   inspectable JSON, and the model does not contain voicebank waveforms.
3. **Voicebank** loads all `oto.ini` files in a bank and resolves the requested
   phonetic context to candidate recordings.
4. **Renderer** places and transforms recorded units according to the prosody
   plan. A later boundary model may correct only the joins between units.

The old JSUT/WORLD and experimental VITS pipelines were removed from the
working tree. They remain available through Git history for comparison only.

## Initial supported scope

- Japanese voicebanks using CV or VCV aliases (CVVC metadata can be loaded,
  but VC insertion is not implemented yet)
- one or more recursively discovered `oto.ini` files
- UTF-8 or Shift_JIS metadata
- PCM WAV source files
- deterministic synthesis for identical inputs and settings

The deterministic renderer and initial JSUT prosody-training baseline are now
implemented. Neural boundary correction comes only after the renderer has
objective tests.

## Quality gates

- Missing and malformed entries are reported, never silently skipped.
- The same input produces byte-identical output.
- Unit placement is tested using synthetic impulses before subjective listening.
- Evaluation keeps prosody error, boundary discontinuity and voice similarity as
  separate measurements.
