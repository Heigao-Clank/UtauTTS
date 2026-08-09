package tts

import (
	"fmt"

	"utautts/internal/audio"
	"utautts/internal/connection"
	"utautts/internal/frontend"
	"utautts/internal/plan"
	"utautts/internal/prosody"
	"utautts/internal/render"
	"utautts/internal/voicebank"
)

type Config struct {
	VoicebankPath       string
	Text                string
	Reading             string
	Tone                string
	MoraDurationMS      float64
	PauseDurationMS     float64
	ReleaseMS           float64
	ProsodyModelPath    string
	IntonationStrength  float64
	Renderer            string
	WorldlinePath       string
	WorldlineBridgePath string
	SelectionMode       voicebank.SelectionMode
	JoinModelPath       string
	JoinScoreScale      float64
}

type Result struct {
	Voicebank *voicebank.Bank
	Plan      *plan.Plan
	Audio     *audio.PCM
}

func Synthesize(cfg Config) (*Result, error) {
	bank, err := voicebank.Load(cfg.VoicebankPath)
	if err != nil {
		return nil, fmt.Errorf("load voicebank: %w", err)
	}
	reading := cfg.Reading
	if reading == "" {
		reading, err = frontend.ToKana(cfg.Text)
		if err != nil {
			return nil, fmt.Errorf("convert text to reading: %w", err)
		}
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		return nil, fmt.Errorf("parse reading: %w", err)
	}
	var joinModel *connection.LearnedModel
	joinCostMode := "handcrafted"
	joinModelVersion := 0
	if cfg.JoinModelPath != "" {
		joinModel, err = connection.LoadLearnedModel(cfg.JoinModelPath)
		if err != nil {
			return nil, fmt.Errorf("load join model: %w", err)
		}
		joinCostMode = "learned"
		joinModelVersion = joinModel.Version
		if cfg.JoinScoreScale > 0 {
			joinModel.ScoreScale = cfg.JoinScoreScale
		}
	}
	if cfg.SelectionMode == voicebank.SelectionTargetOnly {
		joinCostMode = "none"
	}
	selections, err := bank.ResolveWithConfig(morae, voicebank.ResolveConfig{
		Tone: cfg.Tone, Mode: cfg.SelectionMode, JoinModel: joinModel,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve voicebank units: %w", err)
	}
	var predictions []prosody.Prediction
	if cfg.ProsodyModelPath != "" {
		model, loadErr := prosody.LoadModel(cfg.ProsodyModelPath)
		if loadErr != nil {
			return nil, fmt.Errorf("load prosody model: %w", loadErr)
		}
		predictions = model.Predict(morae)
	}
	synthesisPlan, err := plan.Build(bank, reading, morae, selections, plan.Config{
		MoraDurationMS:   cfg.MoraDurationMS,
		PauseDurationMS:  cfg.PauseDurationMS,
		Predictions:      predictions,
		SelectionMode:    cfg.SelectionMode,
		JoinCostMode:     joinCostMode,
		JoinModelVersion: joinModelVersion,
		JoinScoreScale:   joinModelScoreScale(joinModel),
	})
	if err != nil {
		return nil, fmt.Errorf("build synthesis plan: %w", err)
	}
	synthesisPlan.Text = cfg.Text
	pcm, err := render.Render(synthesisPlan, render.Config{
		ReleaseMS:           cfg.ReleaseMS,
		IntonationStrength:  cfg.IntonationStrength,
		Backend:             cfg.Renderer,
		WorldlinePath:       cfg.WorldlinePath,
		WorldlineBridgePath: cfg.WorldlineBridgePath,
	})
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	return &Result{Voicebank: bank, Plan: synthesisPlan, Audio: pcm}, nil
}

func joinModelScoreScale(model *connection.LearnedModel) float64 {
	if model == nil {
		return 0
	}
	return model.ScoreScale
}
