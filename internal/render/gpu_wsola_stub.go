//go:build !windows

package render

import "errors"

func gpuWaveformAvailable() error {
	return errors.New("CUDA waveform renderer is currently available only on Windows")
}

func gpuWaveformLibraryPath() (string, error) {
	return "", gpuWaveformAvailable()
}

func gpuWSOLA(_ []float64, _ int, _ int) ([]float64, error) {
	return nil, gpuWaveformAvailable()
}
