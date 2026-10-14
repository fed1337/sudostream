package transcode

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// VideoEncoder names the ffmpeg video codec selected for HLS output.
type VideoEncoder string

const (
	// EncoderSoftware uses libx264 (no hardware).
	EncoderSoftware VideoEncoder = "libx264"
	// EncoderNVENC uses NVIDIA h264_nvenc (NVIDIA GPU only).
	EncoderNVENC VideoEncoder = "h264_nvenc"
	// EncoderVAAPI uses h264_vaapi (Intel or AMD via /dev/dri).
	EncoderVAAPI VideoEncoder = "h264_vaapi"
	// EncoderQSV uses Intel Quick Sync h264_qsv (Intel GPU only).
	EncoderQSV VideoEncoder = "h264_qsv"
	// EncoderRockchip uses Rockchip RKMPP h264_rkmpp (arm64 VPU).
	EncoderRockchip VideoEncoder = "h264_rkmpp"
)

const (
	pciVendorNVIDIA = "0x10de"
	pciVendorIntel  = "0x8086"
	pciVendorAMD    = "0x1002"
)

type gpuVendor string

const (
	gpuNone    gpuVendor = ""
	gpuNVIDIA  gpuVendor = "nvidia"
	gpuIntel   gpuVendor = "intel"
	gpuAMD     gpuVendor = "amd"
	gpuUnknown gpuVendor = "unknown"
)

var (
	cachedRenderNode     string
	cachedRenderNodeOnce sync.Once

	checkNVIDIAPresent   = nvidiaDevicePresent
	checkRockchipPresent = rockchipDevicePresent
	checkRenderDevice    = firstRenderDevice
	checkGPUVendor       = primaryGPUVendor
)

// ResolveVideoEncoder picks an encoder from the admin preference.
// Missing devices fall back to libx264 (no autodetection chain).
//
//	off      → software
//	qsv      → Intel + renderD* → h264_qsv, else software
//	vaapi    → renderD* (Intel or AMD) → h264_vaapi, else software
//	nvenc    → /dev/nvidia0 → h264_nvenc, else software
//	rockchip → /dev/mpp_service → h264_rkmpp, else software
func ResolveVideoEncoder(pref HwAccel) (VideoEncoder, string) {
	normalized, err := ParseHwAccel(string(pref))
	if err != nil {
		normalized = HwAccelOff
	}

	encoder, render, reason := resolvePreferredEncoder(normalized)
	if reason != "" && normalized != HwAccelOff {
		slog.Info("transcode encoder resolved",
			slog.String("preferred", string(normalized)),
			slog.String("encoder", string(encoder)),
			slog.String("render_node", render),
			slog.String("fallback_reason", reason),
		)
	} else {
		slog.Debug("transcode encoder resolved",
			slog.String("preferred", string(normalized)),
			slog.String("encoder", string(encoder)),
			slog.String("render_node", render),
		)
	}

	return encoder, render
}

//nolint:cyclop // preference×device matrix
func resolvePreferredEncoder(pref HwAccel) (VideoEncoder, string, string) {
	switch pref {
	case HwAccelOff:
		return EncoderSoftware, "", string(HwAccelOff)
	case HwAccelNVENC:
		if checkNVIDIAPresent() {
			return EncoderNVENC, "", ""
		}

		return EncoderSoftware, "", "nvidia device missing"
	case HwAccelVAAPI:
		render := checkRenderDevice()
		if render == "" {
			return EncoderSoftware, "", "render device missing"
		}
		// VA-API works on Intel and AMD GPUs that expose /dev/dri.
		return EncoderVAAPI, render, ""
	case HwAccelQSV:
		render := checkRenderDevice()
		if render == "" {
			return EncoderSoftware, "", "render device missing"
		}
		if checkGPUVendor() != gpuIntel {
			return EncoderSoftware, render, "qsv requires intel gpu"
		}

		return EncoderQSV, render, ""
	case HwAccelRockchip:
		if checkRockchipPresent() {
			return EncoderRockchip, "", ""
		}

		return EncoderSoftware, "", "mpp_service missing"
	default:
		return EncoderSoftware, "", "unknown preference"
	}
}

func nvidiaDevicePresent() bool {
	_, err := os.Stat("/dev/nvidia0")

	return err == nil
}

func rockchipDevicePresent() bool {
	_, err := os.Stat("/dev/mpp_service")

	return err == nil
}

func firstRenderDevice() string {
	cachedRenderNodeOnce.Do(func() {
		cachedRenderNode = discoverRenderDevice()
	})

	return cachedRenderNode
}

func discoverRenderDevice() string {
	entries, err := os.ReadDir("/dev/dri")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "renderD") {
			return filepath.Join("/dev/dri", entry.Name())
		}
	}

	return ""
}

func primaryGPUVendor() gpuVendor {
	matches, err := filepath.Glob("/sys/class/drm/card*/device/vendor")
	if err != nil {
		return gpuNone
	}

	for _, path := range matches {
		if strings.Contains(path, "card") {
			vendor := readPCIVendor(path)
			if vendor != gpuNone && vendor != gpuUnknown {
				return vendor
			}
		}
	}

	return gpuNone
}

func readPCIVendor(path string) gpuVendor {
	raw, err := os.ReadFile(path)
	if err != nil {
		return gpuNone
	}

	return pciIDToVendor(strings.TrimSpace(string(raw)))
}

func pciIDToVendor(id string) gpuVendor {
	switch strings.ToLower(id) {
	case pciVendorNVIDIA:
		return gpuNVIDIA
	case pciVendorIntel:
		return gpuIntel
	case pciVendorAMD:
		return gpuAMD
	case "":
		return gpuNone
	default:
		return gpuUnknown
	}
}
