//go:build windows

package render

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

type cudaWaveformLibrary struct {
	dll       *syscall.DLL
	available *syscall.Proc
	wsola     *syscall.Proc
}

var (
	cudaWaveformOnce sync.Once
	cudaWaveform     cudaWaveformLibrary
	cudaWaveformErr  error
)

func cudaWaveformCandidates() []string {
	var candidates []string
	if configured := os.Getenv("UTAUTTS_WAVEFORM_GPU_LIBRARY"); configured != "" {
		candidates = append(candidates, configured)
	}
	if executable, err := os.Executable(); err == nil {
		directory := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(directory, "runtime", "utautts-waveform-gpu.dll"),
			filepath.Join(directory, "utautts-waveform-gpu.dll"),
			filepath.Join(filepath.Dir(directory), "runtime", "utautts-waveform-gpu.dll"),
		)
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		for directory, depth := workingDirectory, 0; depth < 5; depth++ {
			candidates = append(candidates, filepath.Join(directory, "build", "gpu", "utautts-waveform-gpu.dll"))
			parent := filepath.Dir(directory)
			if parent == directory {
				break
			}
			directory = parent
		}
	}
	return candidates
}

func loadCUDAWaveform() (*cudaWaveformLibrary, error) {
	cudaWaveformOnce.Do(func() {
		for _, candidate := range cudaWaveformCandidates() {
			absolute, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			dll, err := syscall.LoadDLL(absolute)
			if err != nil {
				continue
			}
			available, availableErr := dll.FindProc("UtauTTSGPUAvailable")
			wsola, wsolaErr := dll.FindProc("UtauTTSGPUWSOLA")
			if availableErr != nil || wsolaErr != nil {
				_ = dll.Release()
				continue
			}
			cudaWaveform = cudaWaveformLibrary{dll: dll, available: available, wsola: wsola}
			return
		}
		cudaWaveformErr = errors.New("CUDA waveform backend DLL was not found; run tools/build-waveform-gpu.ps1")
	})
	if cudaWaveformErr != nil {
		return nil, cudaWaveformErr
	}
	return &cudaWaveform, nil
}

func gpuWaveformLibraryPath() (string, error) {
	for _, candidate := range cudaWaveformCandidates() {
		absolute, err := filepath.Abs(candidate)
		if err == nil {
			if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() {
				return absolute, nil
			}
		}
	}
	return "", errors.New("CUDA waveform backend DLL was not found; run tools/build-waveform-gpu.ps1")
}

func gpuWaveformAvailable() error {
	library, err := loadCUDAWaveform()
	if err != nil {
		return err
	}
	errorBuffer := make([]byte, 512)
	available, _, _ := library.available.Call(
		uintptr(unsafe.Pointer(&errorBuffer[0])), uintptr(len(errorBuffer)),
	)
	runtime.KeepAlive(errorBuffer)
	if available == 0 {
		message := string(errorBuffer[:bytesBeforeNUL(errorBuffer)])
		if message == "" {
			message = "unknown CUDA error"
		}
		return fmt.Errorf("CUDA waveform backend is unavailable: %s", message)
	}
	return nil
}

func gpuWSOLA(source []float64, targetFrames, sampleRate int) ([]float64, error) {
	if targetFrames <= 0 || len(source) == 0 {
		return nil, nil
	}
	if len(source) < 16 || targetFrames < 16 {
		return linearResample(source, targetFrames), nil
	}
	library, err := loadCUDAWaveform()
	if err != nil {
		return nil, err
	}
	result := make([]float64, targetFrames)
	errorBuffer := make([]byte, 512)
	ok, _, _ := library.wsola.Call(
		uintptr(unsafe.Pointer(&source[0])), uintptr(len(source)),
		uintptr(unsafe.Pointer(&result[0])), uintptr(targetFrames), uintptr(sampleRate),
		uintptr(unsafe.Pointer(&errorBuffer[0])), uintptr(len(errorBuffer)),
	)
	runtime.KeepAlive(source)
	runtime.KeepAlive(result)
	runtime.KeepAlive(errorBuffer)
	if ok == 0 {
		message := string(errorBuffer[:bytesBeforeNUL(errorBuffer)])
		if message == "" {
			message = "unknown CUDA error"
		}
		return nil, fmt.Errorf("CUDA WSOLA failed: %s", message)
	}
	return result, nil
}

func bytesBeforeNUL(value []byte) int {
	for index, item := range value {
		if item == 0 {
			return index
		}
	}
	return len(value)
}
