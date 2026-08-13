package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"utautts/internal/api"
)

func main() {
	var port int
	var host string
	var config api.Config
	flag.IntVar(&port, "port", 8080, "port")
	flag.StringVar(&host, "host", "127.0.0.1", "host")
	flag.StringVar(&config.VoiceDir, "voice-dir", "voice", "directory containing voicebanks")
	flag.StringVar(&config.ProsodyModelPath, "prosody", "", "optional learned prosody model JSON")
	flag.StringVar(&config.Renderer, "renderer", "", "default renderer plugin ID (default: highest manifest priority)")
	flag.StringVar(&config.WorldlinePath, "worldline", "", "path to worldline library")
	flag.StringVar(&config.WorldlineBridgePath, "worldline-bridge", "", "path to worldline bridge")
	flag.StringVar(&config.WorldlineR2MelPath, "worldline-r2-mel", "", "path to OpenUtau WORLDLINE-R2 mel.onnx")
	flag.StringVar(&config.WorldlineR2VocoderPath, "worldline-r2-vocoder", "", "path to the external PC-NSF-HiFiGAN ONNX model")
	flag.IntVar(&config.OnnxDeviceID, "onnx-device", 0, "DirectML GPU device ID")
	flag.StringVar(&config.OpenJTalkPath, "openjtalk-features", "", "path to Open JTalk feature helper")
	flag.StringVar(&config.OpenJTalkDictionary, "openjtalk-dictionary", "", "path to Open JTalk dictionary")
	flag.StringVar(&config.AuthToken, "auth-token", "", "optional local UI authentication token")
	flag.BoolVar(&config.AllowVoicebankRegistration, "allow-voicebank-registration", false, "allow registration of paths below voice-dir")
	flag.Func("renderer-dir", "renderer plugin directory (repeatable)", func(value string) error {
		config.RendererDirectories = append(config.RendererDirectories, value)
		return nil
	})
	flag.Func("model-dir", "prosody model directory (repeatable)", func(value string) error {
		config.ModelDirectories = append(config.ModelDirectories, value)
		return nil
	})
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + listener.Addr().String()
	fmt.Printf("UTAUTTS_READY=%s\n", url)
	log.Printf("listening on %s", url)
	server := &http.Server{Handler: api.New(config).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 10 * time.Minute, IdleTimeout: 60 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
