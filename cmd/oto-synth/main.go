package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"

    "utautts/internal/audio"
    "utautts/internal/synth"
)

func main() {
    cfg := synth.Config{}

    flag.StringVar(&cfg.OtoPath, "oto", "", "path to oto.ini")
    flag.StringVar(&cfg.Text, "text", "", "aliases separated by spaces or plain kana")
    var outPath string
    flag.StringVar(&outPath, "out", "", "output wav path")
    flag.Float64Var(&cfg.GapMs, "gap", 40, "silence gap between samples (ms)")
    flag.Float64Var(&cfg.CrossMs, "cross", 10, "crossfade length (ms)")
    flag.Float64Var(&cfg.Pitch, "pitch", 1.0, "pitch scale (1.0 = no change)")
    flag.StringVar(&cfg.PredPath, "pred", "", "predictions jsonl (optional)")
    flag.StringVar(&cfg.PlanPath, "plan", "", "synthesis plan jsonl (optional)")
    flag.BoolVar(&cfg.NoCurve, "nocurve", false, "disable F0/RMS curve application")
    flag.Parse()

    if cfg.OtoPath == "" || (cfg.Text == "" && cfg.PlanPath == "") || outPath == "" {
        log.Fatal("-oto, -text (or -plan), -out are required")
    }

    pcm, err := synth.Synthesize(cfg)
    if err != nil {
        log.Fatal(err)
    }

    if err := ensureParentDir(outPath); err != nil {
        log.Fatal(err)
    }
    if err := audio.WriteWav(outPath, pcm); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("wrote %s\n", outPath)
}

func ensureParentDir(path string) error {
    dir := filepath.Dir(path)
    if dir == "." || dir == "" {
        return nil
    }
    if _, err := os.Stat(dir); err == nil {
        return nil
    } else if os.IsNotExist(err) {
        return os.MkdirAll(dir, 0o755)
    } else {
        return err
    }
}
