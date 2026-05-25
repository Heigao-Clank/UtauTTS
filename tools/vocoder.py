import argparse
import json
import os
import urllib.request
from pathlib import Path

import numpy as np
import torch
from torch import nn


def main():
    parser = argparse.ArgumentParser(description="Neural vocoder: WORLD params -> wav via HiFi-GAN")
    parser.add_argument("--sp", help="spectral envelope .npy")
    parser.add_argument("--ap", help="aperiodicity .npy")
    parser.add_argument("--f0", help="F0 .npy")
    parser.add_argument("--sr", type=int, default=24000, help="sample rate")
    parser.add_argument("--out", required=True, help="output wav")
    parser.add_argument("--model-path", default="", help="pretrained HiFi-GAN checkpoint")
    parser.add_argument("--download", action="store_true", help="download pretrained model")
    args = parser.parse_args()

    model = load_vocoder(args.model_path, args.download)
    device = next(model.parameters()).device

    sp = np.load(args.sp) if args.sp else None
    ap = np.load(args.ap) if args.ap else None
    f0 = np.load(args.f0) if args.f0 else None

    if sp is None:
        raise ValueError("--sp is required")

    wav = synthesize_world(model, sp, ap, f0, args.sr, device)
    import soundfile as sf
    os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
    sf.write(args.out, wav.astype(np.float32), args.sr)
    print(f"wrote {args.out} ({len(wav)/args.sr:.1f}s)")


class HiFiGANGenerator(nn.Module):
    def __init__(
        self,
        in_channels: int = 80,
        out_channels: int = 1,
        channels: int = 512,
        kernel_size: int = 7,
        upsample_scales: tuple = (8, 8, 2, 2),
        upsample_kernel_sizes: tuple = (16, 16, 4, 4),
        resblock_kernel_sizes: tuple = (3, 7, 11),
        resblock_dilations: tuple = ((1, 3, 5), (1, 3, 5), (1, 3, 5)),
        use_additional_convs: bool = True,
        bias: bool = True,
        nonlinear_activation: str = "LeakyReLU",
        nonlinear_activation_params: dict = None,
        use_weight_norm: bool = True,
    ):
        super().__init__()
        if nonlinear_activation_params is None:
            nonlinear_activation_params = {"negative_slope": 0.1}

        self.num_kernels = len(resblock_kernel_sizes)
        self.num_upsamples = len(upsample_scales)

        self.conv_pre = nn.Conv1d(in_channels, channels, kernel_size, 1, padding=(kernel_size - 1) // 2, bias=bias)

        self.upsamples = nn.ModuleList()
        self.resblocks = nn.ModuleList()
        for i in range(len(upsample_scales)):
            self.upsamples.append(
                nn.Sequential(
                    getattr(nn, nonlinear_activation)(**nonlinear_activation_params),
                    nn.ConvTranspose1d(
                        channels // (2 ** i),
                        channels // (2 ** (i + 1)),
                        upsample_kernel_sizes[i],
                        upsample_scales[i],
                        padding=(upsample_kernel_sizes[i] - upsample_scales[i]) // 2,
                    ),
                )
            )
            for j in range(len(resblock_kernel_sizes)):
                self.resblocks.append(
                    ResBlock(
                        channels // (2 ** (i + 1)),
                        resblock_kernel_sizes[j],
                        resblock_dilations[j],
                    )
                )

        self.conv_post = nn.Conv1d(channels // (2 ** self.num_upsamples), out_channels, kernel_size, 1, padding=(kernel_size - 1) // 2, bias=bias)

        if use_weight_norm:
            self.apply_weight_norm()

        self.use_additional_convs = use_additional_convs
        if use_additional_convs:
            self.convs_post = nn.ModuleList()
            for _ in range(3):
                self.convs_post.append(
                    nn.Sequential(
                        getattr(nn, nonlinear_activation)(**nonlinear_activation_params),
                        nn.Conv1d(out_channels, out_channels, kernel_size, 1, padding=(kernel_size - 1) // 2, bias=bias),
                    )
                )

    def forward(self, x):
        x = self.conv_pre(x)
        for i in range(self.num_upsamples):
            x = self.upsamples[i](x)
            xs = 0.0
            for j in range(self.num_kernels):
                xs += self.resblocks[i * self.num_kernels + j](x)
            x = xs / self.num_kernels
        x = self.conv_post(x)
        if self.use_additional_convs:
            for conv in self.convs_post:
                x = conv(x) + x
        x = torch.tanh(x)
        return x

    def apply_weight_norm(self):
        def _apply_weight_norm(m):
            if isinstance(m, (nn.Conv1d, nn.ConvTranspose1d)):
                nn.utils.weight_norm(m)
        self.apply(_apply_weight_norm)

    def remove_weight_norm(self):
        def _remove_weight_norm(m):
            try:
                nn.utils.remove_weight_norm(m)
            except ValueError:
                pass
        self.apply(_remove_weight_norm)


class ResBlock(nn.Module):
    def __init__(self, channels, kernel_size, dilations):
        super().__init__()
        self.convs = nn.ModuleList()
        for d in dilations:
            padding = (kernel_size * d - d) // 2
            self.convs.append(
                nn.Sequential(
                    nn.LeakyReLU(0.1),
                    nn.Conv1d(channels, channels, kernel_size, 1, padding=padding, dilation=d),
                    nn.LeakyReLU(0.1),
                    nn.Conv1d(channels, channels, kernel_size, 1, padding=padding, dilation=1),
                )
            )

    def forward(self, x):
        for conv in self.convs:
            x = conv(x) + x
        return x


def load_vocoder(model_path: str = "", download: bool = False) -> HiFiGANGenerator:
    if not model_path:
        default_path = os.path.join(os.path.dirname(__file__), "..", "data", "vocoder", "hifigan_v1.pth")
        model_path = os.path.normpath(default_path)

    model = HiFiGANGenerator()

    if download and not os.path.exists(model_path):
        print("Downloading pretrained HiFi-GAN model...")
        download_model(model_path)

    if os.path.exists(model_path):
        state = torch.load(model_path, map_location="cpu")
        if "state_dict" in state:
            state = state["state_dict"]
        if "generator" in state:
            state = state["generator"]
        new_state = {}
        for k, v in state.items():
            if k.startswith("module."):
                k = k[7:]
            new_state[k] = v
        model.load_state_dict(new_state, strict=False)
        print(f"loaded vocoder from {model_path}")
    else:
        print(f"warning: no vocoder model at {model_path}, using untrained model")

    model.eval()
    return model


def download_model(dst: str):
    os.makedirs(os.path.dirname(dst) or ".", exist_ok=True)
    urls = [
        "https://github.com/kan-bayashi/ParallelWaveGAN/releases/download/v0.5.3/jsut_parallel_wavegan.v3.tar.gz",
    ]
    for url in urls:
        try:
            print(f"  trying {url}...")
            urllib.request.urlretrieve(url, dst + ".tar.gz")
            import tarfile
            with tarfile.open(dst + ".tar.gz") as tar:
                for member in tar.getmembers():
                    if "checkpoint" in member.name and member.name.endswith(".pkl"):
                        tar.extract(member, os.path.dirname(dst))
                        src = os.path.join(os.path.dirname(dst), member.name)
                        state = torch.load(src, map_location="cpu")
                        gen_state = state["model"]["generator"]
                        torch.save(gen_state, dst)
                        print(f"saved to {dst}")
                        return
            print("  no checkpoint found in archive")
        except Exception as e:
            print(f"  failed: {e}")
    raise RuntimeError("could not download model")


def world_sp_to_mel(sp: np.ndarray, sr: int, n_fft: int = 1024, n_mels: int = 80, fmin: float = 80.0, fmax: float = 7600.0) -> np.ndarray:
    sp_dim = sp.shape[1]
    fft_size = (sp_dim - 1) * 2

    mel_freqs = np.linspace(
        2595 * np.log10(1 + fmin / 700),
        2595 * np.log10(1 + min(fmax, sr / 2) / 700),
        n_mels,
    )
    mel_bins = 700 * (10 ** (mel_freqs / 2595) - 1)
    mel_bin_indices = np.floor((fft_size + 1) * mel_bins / sr).astype(int)

    mel = np.zeros((sp.shape[0], n_mels), dtype=np.float64)
    for i in range(n_mels):
        if mel_bin_indices[i] < sp_dim:
            mel[:, i] = sp[:, mel_bin_indices[i]]
    mel = np.log(np.maximum(mel, 1e-5))
    return mel.astype(np.float32)


def synthesize_world(model: HiFiGANGenerator, sp: np.ndarray, ap: np.ndarray, f0: np.ndarray, sr: int, device: torch.device) -> np.ndarray:
    import pyworld as pw

    if f0 is None or ap is None:
        y = pw.synthesize(
            f0.astype(np.float64) if f0 is not None else np.zeros(sp.shape[0]),
            sp,
            ap if ap is not None else np.ones_like(sp),
            sr,
        )
        return y

    y_world = pw.synthesize(f0.astype(np.float64), sp, ap, sr)
    mel = world_sp_to_mel(sp, sr)
    mel_t = torch.from_numpy(mel).unsqueeze(0).transpose(1, 2).to(device)
    with torch.no_grad():
        wav = model(mel_t).squeeze(0).squeeze(0).cpu().numpy()
    wav = np.nan_to_num(wav, nan=0.0)
    peak = np.abs(wav).max()
    if peak > 0:
        wav *= 0.95 / peak
    return wav


if __name__ == "__main__":
    main()
