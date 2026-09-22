package transcode

import (
	"fmt"
	"strings"
)

// ToneMapMode selects the HDR→SDR filter path for one encode run.
type ToneMapMode string

const (
	// ToneMapNone skips tone mapping (SDR source or admin disabled).
	ToneMapNone ToneMapMode = ""
	// ToneMapCPU uses jellyfin-ffmpeg tonemapx (software / VAAPI-Intel / QSV).
	ToneMapCPU ToneMapMode = "cpu"
	// ToneMapCUDA uses jellyfin-ffmpeg tonemap_cuda.
	ToneMapCUDA ToneMapMode = "cuda"
	// ToneMapQSVVPP uses vpp_qsv tonemap=1 (unused when admin algo is required).
	ToneMapQSVVPP ToneMapMode = "qsv_vpp"
	// ToneMapVulkan uses libplacebo (AMD + VAAPI path).
	ToneMapVulkan ToneMapMode = "vulkan"
	// ToneMapOpenCL uses tonemap_opencl (Rockchip OpenCL path).
	ToneMapOpenCL ToneMapMode = "opencl"
)

// ResolveToneMapMode picks a tonemap path for the active encoder and GPU vendor.
//
// needsToneMap should already include the admin gate:
// meta.NeedsToneMap() && settings.ToneMappingEnabled.
//
// Hard rule from FI-5: never VAAPI VPP — VA-API often fronts AMD; VPP is Intel-centric.
// QSV uses CPU tonemapx so the selected algorithm is honored (VPP has no algo string).
func ResolveToneMapMode(encoder VideoEncoder, vendor gpuVendor, needsToneMap bool) ToneMapMode {
	if !needsToneMap {
		return ToneMapNone
	}

	switch encoder {
	case EncoderNVENC:
		return ToneMapCUDA
	case EncoderQSV:
		return ToneMapCPU
	case EncoderVAAPI:
		if vendor == gpuAMD {
			return ToneMapVulkan
		}

		return ToneMapCPU
	case EncoderRockchip:
		return ToneMapOpenCL
	case EncoderSoftware:
		return ToneMapCPU
	default:
		return ToneMapCPU
	}
}

func tonemapAlgoOrDefault(algo ToneMappingAlgorithm) ToneMappingAlgorithm {
	if algo == "" {
		return ToneMappingBT2390
	}

	return algo
}

// vulkanTonemapAlgo maps admin algo names to libplacebo tonemapping= values.
func vulkanTonemapAlgo(algo ToneMappingAlgorithm) string {
	algo = tonemapAlgoOrDefault(algo)
	if algo == ToneMappingBT2390 {
		return "bt.2390"
	}

	return string(algo)
}

// cpuTonemapFilter is the jellyfin-ffmpeg software HDR→SDR path (tonemapx).
//
// Do not use lavfi `tonemap=tonemap=bt2390`: stock `tonemap` only knows hable/reinhard/etc.
// jellyfin-ffmpeg exposes BT.2390 on `tonemapx` (and HW filters), matching Jellyfin EncodingHelper.
func cpuTonemapFilter(algo ToneMappingAlgorithm) string {
	return fmt.Sprintf(
		"tonemapx=tonemap=%s:desat=0:peak=100:t=bt709:m=bt709:p=bt709:format=yuv420p",
		tonemapAlgoOrDefault(algo),
	)
}

// hwTonemapFilter mirrors Jellyfin EncodingHelper.GetHwTonemapFilter for cuda/opencl.
func hwTonemapFilter(suffix, format string, algo ToneMappingAlgorithm) string {
	if format == "" {
		format = "nv12"
	}

	return fmt.Sprintf(
		"tonemap_%s=format=%s:p=bt709:t=bt709:m=bt709:tonemap=%s:peak=100:desat=0",
		suffix,
		format,
		tonemapAlgoOrDefault(algo),
	)
}

func vulkanTonemapFilter(format string, algo ToneMappingAlgorithm) string {
	if format == "" {
		format = "nv12"
	}

	return "libplacebo=upscaler=none:downscaler=none:format=" + format +
		":tonemapping=" + vulkanTonemapAlgo(algo) +
		":peak_detect=0:color_primaries=bt709:color_trc=bt709:colorspace=bt709"
}

func qsvVPPTonemapScale(height int) string {
	return fmt.Sprintf(
		"vpp_qsv=w=-2:h=min(%d\\,ih):format=nv12:tonemap=1:async_depth=2",
		height,
	)
}

func joinFilters(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}

	return strings.Join(out, ",")
}
