#include <cuda_runtime.h>

#include <cmath>
#include <cfloat>
#include <cstdio>
#include <vector>

#ifdef _WIN32
#define UTAUTTS_EXPORT extern "C" __declspec(dllexport)
#else
#define UTAUTTS_EXPORT extern "C" __attribute__((visibility("default")))
#endif

namespace
{
    thread_local char last_error[512] = {};
    constexpr float pi = 3.14159265358979323846f;

    void set_error(const char *operation, cudaError_t error)
    {
        std::snprintf(last_error, sizeof(last_error), "%s: %s", operation, cudaGetErrorString(error));
    }

    void copy_error(char *destination, int capacity)
    {
        if (destination && capacity > 0)
        {
            std::snprintf(destination, static_cast<size_t>(capacity), "%s", last_error);
        }
    }

    __device__ int clamp_int(int value, int low, int high)
    {
        return max(low, min(high, value));
    }

    __global__ void wsola_kernel(const float *source, int source_frames, float *accumulator, float *weights, float *result, int target_frames, int sample_rate)
    {
        __shared__ float scores[512];
        __shared__ int candidates[512];
        __shared__ int chosen;

        int window = min(min((40 * sample_rate + 500) / 1000, source_frames), target_frames);
        if (window & 1)
            --window;
        int hop = max(1, window / 2);
        int search = max(1, min((5 * sample_rate + 500) / 1000, window / 4));
        int max_start = max(0, source_frames - window);
        float ratio = static_cast<float>(source_frames) / static_cast<float>(target_frames);
        int previous_source = 0;

        for (int output = 0; output < target_frames; output += hop)
        {
            int expected = min(static_cast<int>(floorf(static_cast<float>(output) * ratio + 0.5f)), max_start);
            int low = max(0, expected - search);
            int high = min(max_start, expected + search);
            int count = high - low + 1;

            if (output == 0)
            {
                if (threadIdx.x == 0)
                    chosen = expected;
            }
            else
            {
                int reference = clamp_int(previous_source + hop, 0, max_start);
                float best_score = -FLT_MAX;
                int best_candidate = high + 1;
                for (int candidate_index = static_cast<int>(threadIdx.x); candidate_index < count; candidate_index += blockDim.x)
                {
                    int candidate = low + candidate_index;
                    int length = min(window / 2, min(source_frames - reference, source_frames - candidate));
                    if (length >= 4)
                    {
                        float numerator = 0.0f;
                        float left_energy = 0.0f;
                        float right_energy = 0.0f;
                        for (int index = 0; index < length; ++index)
                        {
                            float left = source[reference + index];
                            float right = source[candidate + index];
                            numerator += left * right;
                            left_energy += left * left;
                            right_energy += right * right;
                        }
                        float score = numerator / (sqrtf(left_energy * right_energy) + 1e-12f);
                        if (score > best_score || (score == best_score && candidate < best_candidate))
                        {
                            best_score = score;
                            best_candidate = candidate;
                        }
                    }
                }
                scores[threadIdx.x] = best_score;
                candidates[threadIdx.x] = best_candidate;
                __syncthreads();
                for (int stride = blockDim.x / 2; stride > 0; stride >>= 1)
                {
                    if (threadIdx.x < stride)
                    {
                        float right_score = scores[threadIdx.x + stride];
                        int right_candidate = candidates[threadIdx.x + stride];
                        if (right_score > scores[threadIdx.x] ||
                            (right_score == scores[threadIdx.x] && right_candidate < candidates[threadIdx.x]))
                        {
                            scores[threadIdx.x] = right_score;
                            candidates[threadIdx.x] = right_candidate;
                        }
                    }
                    __syncthreads();
                }
                if (threadIdx.x == 0)
                    chosen = candidates[0];
            }
            __syncthreads();
            for (int index = static_cast<int>(threadIdx.x); index < window && output + index < target_frames + window; index += blockDim.x)
            {
                float weight = 0.5f - 0.5f * cosf(2.0f * pi * static_cast<float>(index + 1) / static_cast<float>(window + 1));
                accumulator[output + index] += source[chosen + index] * weight;
                weights[output + index] += weight;
            }
            __syncthreads();
            previous_source = chosen;
        }
        for (int index = static_cast<int>(threadIdx.x); index < target_frames; index += blockDim.x)
        {
            result[index] = weights[index] > 1e-12f ? accumulator[index] / weights[index] : 0.0f;
        }
    }

    __global__ void faithful_mix_kernel(
        const float *samples, const int *sample_offsets, const int *sample_lengths,
        const int *starts, const int *skips, const int *visible_lengths,
        const double *envelope_x, const double *envelope_y, int sample_rate,
        float *result, int result_length)
    {
        int unit = static_cast<int>(blockIdx.x);
        int skip = skips[unit];
        int visible_length = visible_lengths[unit];
        for (int visible = static_cast<int>(threadIdx.x); visible < visible_length; visible += blockDim.x)
        {
            int local = visible + skip;
            int output = starts[unit] + visible;
            if (local < 0 || local < skip || local >= sample_lengths[unit] ||
                output < 0 || output >= result_length)
            {
                continue;
            }
            const double *x = envelope_x + unit * 5;
            const double *y = envelope_y + unit * 5;
            double shift = -x[0];
            int next = 0;
            while (next < 5 && static_cast<double>(local) >
                                   (x[next] + shift) * sample_rate / 1000.0 + skip)
            {
                ++next;
            }
            double gain;
            if (next == 0)
            {
                gain = y[0];
            }
            else if (next >= 5)
            {
                gain = y[4];
            }
            else
            {
                double left_sample = (x[next - 1] + shift) * sample_rate / 1000.0 + skip;
                double right_sample = (x[next] + shift) * sample_rate / 1000.0 + skip;
                gain = left_sample >= right_sample ? y[next - 1] : y[next - 1] + (y[next] - y[next - 1]) * (static_cast<double>(local) - left_sample) / (right_sample - left_sample);
            }
            atomicAdd(result + output, samples[sample_offsets[unit] + local] * static_cast<float>(gain));
        }
    }
}

UTAUTTS_EXPORT int UtauTTSGPUAvailable(char *error_output, int error_capacity)
{
    int count = 0;
    cudaError_t error = cudaGetDeviceCount(&count);
    if (error != cudaSuccess)
    {
        set_error("cudaGetDeviceCount", error);
        copy_error(error_output, error_capacity);
        return 0;
    }
    if (count == 0)
    {
        std::snprintf(last_error, sizeof(last_error), "no CUDA device found");
        copy_error(error_output, error_capacity);
        return 0;
    }
    last_error[0] = '\0';
    copy_error(error_output, error_capacity);
    return 1;
}

UTAUTTS_EXPORT int UtauTTSGPUWSOLA(const double *source, int source_frames,
                                   double *result, int target_frames, int sample_rate,
                                   char *error_output, int error_capacity)
{
    if (!source || !result || source_frames < 16 || target_frames < 16 || sample_rate <= 0)
    {
        std::snprintf(last_error, sizeof(last_error), "invalid WSOLA input");
        copy_error(error_output, error_capacity);
        return 0;
    }
    cudaStream_t stream = nullptr;
    float *device_source = nullptr;
    float *device_accumulator = nullptr;
    float *device_weights = nullptr;
    float *device_result = nullptr;
    std::vector<float> host_source(source_frames);
    std::vector<float> host_result(target_frames);
    for (int index = 0; index < source_frames; ++index)
        host_source[index] = static_cast<float>(source[index]);
    const size_t source_bytes = static_cast<size_t>(source_frames) * sizeof(float);
    const size_t work_frames = static_cast<size_t>(target_frames) +
                               static_cast<size_t>(min(source_frames, (40 * sample_rate + 500) / 1000));
    const size_t work_bytes = work_frames * sizeof(float);
    const size_t result_bytes = static_cast<size_t>(target_frames) * sizeof(float);
    cudaError_t error = cudaStreamCreateWithFlags(&stream, cudaStreamNonBlocking);
    if (error == cudaSuccess)
        error = cudaMallocAsync(&device_source, source_bytes, stream);
    if (error == cudaSuccess)
        error = cudaMallocAsync(&device_accumulator, work_bytes, stream);
    if (error == cudaSuccess)
        error = cudaMallocAsync(&device_weights, work_bytes, stream);
    if (error == cudaSuccess)
        error = cudaMallocAsync(&device_result, result_bytes, stream);
    if (error == cudaSuccess)
        error = cudaMemcpyAsync(device_source, host_source.data(), source_bytes, cudaMemcpyHostToDevice, stream);
    if (error == cudaSuccess)
        error = cudaMemsetAsync(device_accumulator, 0, work_bytes, stream);
    if (error == cudaSuccess)
        error = cudaMemsetAsync(device_weights, 0, work_bytes, stream);
    if (error == cudaSuccess)
    {
        wsola_kernel<<<1, 512, 0, stream>>>(device_source, source_frames, device_accumulator,
                                            device_weights, device_result, target_frames, sample_rate);
        error = cudaGetLastError();
    }
    if (error == cudaSuccess)
        error = cudaMemcpyAsync(host_result.data(), device_result, result_bytes, cudaMemcpyDeviceToHost, stream);
    if (device_result)
        cudaFreeAsync(device_result, stream);
    if (device_weights)
        cudaFreeAsync(device_weights, stream);
    if (device_accumulator)
        cudaFreeAsync(device_accumulator, stream);
    if (device_source)
        cudaFreeAsync(device_source, stream);
    if (stream)
    {
        cudaError_t sync_error = cudaStreamSynchronize(stream);
        if (error == cudaSuccess)
            error = sync_error;
        cudaStreamDestroy(stream);
    }
    if (error != cudaSuccess)
    {
        set_error("CUDA WSOLA", error);
        copy_error(error_output, error_capacity);
        return 0;
    }
    for (int index = 0; index < target_frames; ++index)
        result[index] = static_cast<double>(host_result[index]);
    last_error[0] = '\0';
    copy_error(error_output, error_capacity);
    return 1;
}

UTAUTTS_EXPORT int UtauTTSGPUFaithfulMix(
    const float *samples, int sample_count, const int *sample_offsets,
    const int *sample_lengths, const int *starts, const int *skips,
    const int *visible_lengths, const double *envelope_x,
    const double *envelope_y, int unit_count, int sample_rate,
    float *result, int result_length, char *error_output, int error_capacity)
{
    if (!samples || sample_count <= 0 || !sample_offsets || !sample_lengths ||
        !starts || !skips || !visible_lengths || !envelope_x || !envelope_y ||
        unit_count <= 0 || sample_rate <= 0 || !result || result_length <= 0)
    {
        std::snprintf(last_error, sizeof(last_error), "invalid faithful mix input");
        copy_error(error_output, error_capacity);
        return 0;
    }

    float *device_samples = nullptr;
    float *device_result = nullptr;
    int *device_offsets = nullptr;
    int *device_lengths = nullptr;
    int *device_starts = nullptr;
    int *device_skips = nullptr;
    int *device_visible = nullptr;
    double *device_envelope_x = nullptr;
    double *device_envelope_y = nullptr;
    size_t samples_bytes = static_cast<size_t>(sample_count) * sizeof(float);
    size_t units_bytes = static_cast<size_t>(unit_count) * sizeof(int);
    size_t envelope_bytes = static_cast<size_t>(unit_count) * 5 * sizeof(double);
    size_t result_bytes = static_cast<size_t>(result_length) * sizeof(float);

    cudaError_t error = cudaMalloc(&device_samples, samples_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_result, result_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_offsets, units_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_lengths, units_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_starts, units_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_skips, units_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_visible, units_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_envelope_x, envelope_bytes);
    if (error == cudaSuccess)
        error = cudaMalloc(&device_envelope_y, envelope_bytes);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_samples, samples, samples_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_offsets, sample_offsets, units_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_lengths, sample_lengths, units_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_starts, starts, units_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_skips, skips, units_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_visible, visible_lengths, units_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_envelope_x, envelope_x, envelope_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemcpy(device_envelope_y, envelope_y, envelope_bytes, cudaMemcpyHostToDevice);
    if (error == cudaSuccess)
        error = cudaMemset(device_result, 0, result_bytes);
    if (error == cudaSuccess)
    {
        faithful_mix_kernel<<<unit_count, 256>>>(
            device_samples, device_offsets, device_lengths, device_starts,
            device_skips, device_visible, device_envelope_x, device_envelope_y,
            sample_rate, device_result, result_length);
        error = cudaGetLastError();
    }
    if (error == cudaSuccess)
        error = cudaMemcpy(result, device_result, result_bytes, cudaMemcpyDeviceToHost);

    if (device_envelope_y)
        cudaFree(device_envelope_y);
    if (device_envelope_x)
        cudaFree(device_envelope_x);
    if (device_visible)
        cudaFree(device_visible);
    if (device_skips)
        cudaFree(device_skips);
    if (device_starts)
        cudaFree(device_starts);
    if (device_lengths)
        cudaFree(device_lengths);
    if (device_offsets)
        cudaFree(device_offsets);
    if (device_result)
        cudaFree(device_result);
    if (device_samples)
        cudaFree(device_samples);
    if (error != cudaSuccess)
    {
        set_error("CUDA faithful mix", error);
        copy_error(error_output, error_capacity);
        return 0;
    }
    last_error[0] = '\0';
    copy_error(error_output, error_capacity);
    return 1;
}
