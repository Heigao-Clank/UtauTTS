package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"utautts/internal/engine"
)

func main() {
	cfg := engine.Config{}

	exe, _ := os.Executable()
	defaultTools := filepath.Dir(exe)

	flag.StringVar(&cfg.OtoPath, "oto", "", "path to oto.ini")
	flag.StringVar(&cfg.Text, "text", "", "input text")
	flag.StringVar(&cfg.OutPath, "out", "", "output wav path")
	flag.StringVar(&cfg.ModelPath, "model", "", "DNN model path (.pth)")
	flag.StringVar(&cfg.Python, "python", "python", "python executable")
	flag.StringVar(&cfg.ToolsDir, "tools", defaultTools, "tools directory")
	flag.Parse()

	if cfg.OtoPath == "" || cfg.Text == "" || cfg.OutPath == "" || cfg.ModelPath == "" {
		log.Fatal("--oto, --text, --out, --model are required")
	}

	if err := engine.Synthesize(cfg); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("wrote %s\n", cfg.OutPath)
}
