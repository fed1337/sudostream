package transcode

import (
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestNeedsToneMap_HDRSignals(t *testing.T) {
	t.Parallel()

	allure.Test(t, "NeedsToneMap detects PQ/HLG/BT2020/10-bit", func(a *allure.Context) {
		t := a.T()
		if (SourceInfo{}).NeedsToneMap() {
			t.Fatal("SDR defaults must not tone-map")
		}
		if !(SourceInfo{ColorTransfer: colorTransferPQ}).NeedsToneMap() {
			t.Fatal("PQ transfer requires tone-map")
		}
		if !(SourceInfo{ColorPrimaries: colorPrimaries2020}).NeedsToneMap() {
			t.Fatal("bt2020 requires tone-map")
		}
		if !(SourceInfo{PixFmt: "yuv420p10le"}).NeedsToneMap() {
			t.Fatal("10-bit pix_fmt requires tone-map")
		}
	})
}

func TestResolveToneMapMode_ByEncoder(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tone map mode follows encoder and AMD VAAPI rule", func(a *allure.Context) {
		t := a.T()
		if ResolveToneMapMode(EncoderNVENC, gpuNVIDIA, false) != ToneMapNone {
			t.Fatal("SDR → none")
		}
		if ResolveToneMapMode(EncoderNVENC, gpuNVIDIA, true) != ToneMapCUDA {
			t.Fatal("NVENC HDR → cuda")
		}
		if ResolveToneMapMode(EncoderQSV, gpuIntel, true) != ToneMapCPU {
			t.Fatal("QSV HDR → cpu tonemapx (algo fidelity)")
		}
		if ResolveToneMapMode(EncoderVAAPI, gpuAMD, true) != ToneMapVulkan {
			t.Fatal("VAAPI AMD HDR → vulkan")
		}
		if ResolveToneMapMode(EncoderVAAPI, gpuIntel, true) != ToneMapCPU {
			t.Fatal("VAAPI Intel HDR → cpu (never VAAPI VPP)")
		}
		if ResolveToneMapMode(EncoderRockchip, gpuNone, true) != ToneMapOpenCL {
			t.Fatal("rkmpp HDR → opencl")
		}
	})
}

func TestAppendVideoEncodeArgs_VAAPIScaleAndTonemap(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"VAAPI ladder uses scale_vaapi; CPU tonemap prepended for HDR",
		func(a *allure.Context) {
			t := a.T()
			rung := FastStartRung(720)
			plain := strings.Join(
				appendVideoEncodeArgs(nil, EncoderVAAPI, rung, ToneMapNone, ToneMappingBT2390),
				" ",
			)
			if !strings.Contains(plain, "scale_vaapi") {
				t.Fatalf("expected scale_vaapi, got: %s", plain)
			}

			hdr := strings.Join(
				appendVideoEncodeArgs(nil, EncoderVAAPI, rung, ToneMapCPU, ToneMappingHable),
				" ",
			)
			if !strings.Contains(hdr, "tonemapx=tonemap=hable") ||
				!strings.Contains(hdr, "scale_vaapi") {
				t.Fatalf("expected tonemapx hable + scale_vaapi, got: %s", hdr)
			}
			if strings.Contains(hdr, "tonemap_vaapi") {
				t.Fatal("must never use VAAPI VPP tonemap")
			}
		},
	)
}

//nolint:cyclop // encoder × algo matrix in one scenario
func TestAppendVideoEncodeArgs_HWTonemapFilters(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HW tonemap filter strings for CUDA QSV Vulkan OpenCL", func(a *allure.Context) {
		t := a.T()
		rung := FastStartRung(720)

		cuda := strings.Join(
			appendVideoEncodeArgs(nil, EncoderNVENC, rung, ToneMapCUDA, ToneMappingReinhard),
			" ",
		)
		if !strings.Contains(cuda, "tonemap_cuda") ||
			!strings.Contains(cuda, "tonemap=reinhard") {
			t.Fatalf("NVENC CUDA tonemap missing: %s", cuda)
		}

		qsv := strings.Join(
			appendVideoEncodeArgs(nil, EncoderQSV, rung, ToneMapCPU, ToneMappingMobius),
			" ",
		)
		if !strings.Contains(qsv, "tonemapx=tonemap=mobius") ||
			!strings.Contains(qsv, "scale_qsv") {
			t.Fatalf("QSV CPU tonemapx missing: %s", qsv)
		}

		// Legacy VPP path still builds if forced (unused by ResolveToneMapMode).
		qsvVPP := strings.Join(
			appendVideoEncodeArgs(nil, EncoderQSV, rung, ToneMapQSVVPP, ToneMappingBT2390),
			" ",
		)
		if !strings.Contains(qsvVPP, "vpp_qsv") || !strings.Contains(qsvVPP, "tonemap=1") {
			t.Fatalf("QSV VPP tonemap missing: %s", qsvVPP)
		}

		vulkan := strings.Join(
			appendVideoEncodeArgs(nil, EncoderVAAPI, rung, ToneMapVulkan, ToneMappingBT2390),
			" ",
		)
		if !strings.Contains(vulkan, "libplacebo") ||
			!strings.Contains(vulkan, "tonemapping=bt.2390") ||
			!strings.Contains(vulkan, "scale_vaapi") {
			t.Fatalf("VAAPI Vulkan tonemap missing: %s", vulkan)
		}

		opencl := strings.Join(
			appendVideoEncodeArgs(nil, EncoderRockchip, rung, ToneMapOpenCL, ToneMappingHable),
			" ",
		)
		if !strings.Contains(opencl, "tonemap_opencl") ||
			!strings.Contains(opencl, "tonemap=hable") {
			t.Fatalf("Rockchip OpenCL tonemap missing: %s", opencl)
		}

		rkCPU := strings.Join(
			appendVideoEncodeArgs(nil, EncoderRockchip, rung, ToneMapCPU, ToneMappingBT2390),
			" ",
		)
		if !strings.Contains(rkCPU, "tonemapx=tonemap=bt2390") {
			t.Fatalf("Rockchip CPU tonemapx missing: %s", rkCPU)
		}
	})
}
