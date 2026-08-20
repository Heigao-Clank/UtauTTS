// Package synthは、Qt GUIバックエンドとHTTPサーバが共有するトランスポート非依存の
// 合成オーケストレータである。ボイスバンク・プロソディモデル・レンダラプラグインを解決し、
// ttsパッケージにディスパッチする。呼び出し側は自身のリクエストのデコード・検証・
// レスポンスエンコードのみを扱えばよい。
package synth

import (
	"context"
	"errors"
	"fmt"

	"utautts/internal/plugin"
	"utautts/internal/prosody"
	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

// ErrUnavailableは解決失敗（ボイスバンク・プロソディモデル・レンダラプラグインが見つからない）
// を表す。HTTP呼び出し側は合成失敗とは異なるクライアントエラーを返せる。
var ErrUnavailable = errors.New("unavailable")

// Requestは、共有のトランスポート非依存な合成・プレビューリクエストである。
type Request struct {
	Text               string
	Kana               string
	VoicebankID        string
	Tone               string
	ModelID            string
	Renderer           string
	AliasPolicy        voicebank.AliasPolicy
	Dictionary         map[string]string
	MoraDurationMS     float64
	PauseDurationMS    float64
	MoraDurationsMS    []float64
	IntonationStrength float64
	ApplyPitch         bool
	ManualPitch        *prosody.ManualPitchFile
}

// VoicebankResolverはボイスバンクIDをそのルートパスにマッピングする。空のIDは
// デフォルトのボイスバンクを選択する。
type VoicebankResolver interface {
	Resolve(id string) (path string, ok bool)
}

// Serviceはプラグインカタログとボイスバンクリゾルバに対してリクエスト入力を解決し、
// ttsパッケージにディスパッチする。
type Service struct {
	catalog             *plugin.Catalog
	renderer            string
	worldlinePath       string
	worldlineBridgePath string
	openJTalkPath       string
	openJTalkDictionary string
	voicebanks          VoicebankResolver
}

func NewService(catalog *plugin.Catalog, renderer, worldlinePath, worldlineBridgePath, openJTalkPath, openJTalkDictionary string, voicebanks VoicebankResolver) *Service {
	return &Service{
		catalog: catalog, renderer: renderer,
		worldlinePath: worldlinePath, worldlineBridgePath: worldlineBridgePath,
		openJTalkPath: openJTalkPath, openJTalkDictionary: openJTalkDictionary,
		voicebanks: voicebanks,
	}
}

// Synthesizeはリクエストを解決して音声をレンダリングし、実際に使用されたレンダラIDを返す。
func (s *Service) Synthesize(request Request) (*tts.Result, string, error) {
	return s.SynthesizeContext(context.Background(), request)
}

func (s *Service) SynthesizeContext(ctx context.Context, request Request) (*tts.Result, string, error) {
	cfg, rendererID, err := s.config(request, true)
	if err != nil {
		return nil, "", err
	}
	cfg.Context = ctx
	result, err := tts.Synthesize(cfg)
	if err != nil {
		return nil, "", err
	}
	return result, rendererID, nil
}

// PredictProsodyはリクエストを解決してプロソディをプレビューする。音声の合成や
// ボイスバンクの読み込みは行わない。
func (s *Service) PredictProsody(request Request) (*tts.ProsodyPreview, string, error) {
	cfg, rendererID, err := s.config(request, false)
	if err != nil {
		return nil, "", err
	}
	preview, err := tts.PredictProsody(cfg)
	if err != nil {
		return nil, "", err
	}
	return preview, rendererID, nil
}

func (s *Service) config(request Request, requireVoicebank bool) (tts.Config, string, error) {
	modelPath, err := s.modelPath(request.ModelID)
	if err != nil {
		return tts.Config{}, "", err
	}
	cfg := tts.Config{
		Text:                    request.Text,
		Reading:                 request.Kana,
		Dictionary:              request.Dictionary,
		Tone:                    request.Tone,
		AliasPolicy:             request.AliasPolicy,
		MoraDurationMS:          request.MoraDurationMS,
		PauseDurationMS:         request.PauseDurationMS,
		MoraDurationsMS:         request.MoraDurationsMS,
		ProsodyModelPath:        modelPath,
		ManualPitch:             request.ManualPitch,
		IntonationStrength:      request.IntonationStrength,
		ApplyPitch:              request.ApplyPitch,
		OpenJTalkPath:           s.openJTalkPath,
		OpenJTalkDictionaryPath: s.openJTalkDictionary,
	}
	if requireVoicebank {
		if s.voicebanks == nil {
			return tts.Config{}, "", fmt.Errorf("%w: voicebank resolver is not configured", ErrUnavailable)
		}
		voicebankPath, ok := s.voicebanks.Resolve(request.VoicebankID)
		if !ok {
			return tts.Config{}, "", fmt.Errorf("%w: voicebank not found", ErrUnavailable)
		}
		cfg.VoicebankPath = voicebankPath
	}
	rendererID := s.rendererID(request.Renderer)
	if err := tts.ApplyRenderer(&cfg, s.catalog, rendererID, s.worldlinePath, s.worldlineBridgePath); err != nil {
		return tts.Config{}, "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return cfg, rendererID, nil
}

func (s *Service) rendererID(requested string) string {
	if requested != "" {
		return requested
	}
	return s.renderer
}

// modelPathはプロソディモデルIDをファイルパスに解決する。空または"none"のIDは
// モデルなしを選択する。
func (s *Service) modelPath(id string) (string, error) {
	if id == "" || id == "none" {
		return "", nil
	}
	model, found := s.catalog.Model(id)
	if !found {
		return "", fmt.Errorf("%w: prosody model %q not found", ErrUnavailable, id)
	}
	return model.Path, nil
}

// ModelAvailableは、リクエストがプロソディモデルを選択するかどうかを返す。呼び出し側が返す
// "prosody_model_applied"フィールドと一致する。
func (s *Service) ModelAvailable(id string) bool {
	if id == "" || id == "none" {
		return false
	}
	_, found := s.catalog.Model(id)
	return found
}
