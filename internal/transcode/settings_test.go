package transcode

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const testRenderNode = "/dev/dri/renderD128"

func TestParseHwAccel(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parses and rejects hwAccel values", func(a *allure.Context) {
		t := a.T()
		cases := []struct {
			raw     string
			want    HwAccel
			wantErr bool
		}{
			{raw: string(HwAccelOff), want: HwAccelOff},
			{raw: "", want: HwAccelOff},
			{raw: "QSV", want: HwAccelQSV},
			{raw: "vaapi", want: HwAccelVAAPI},
			{raw: "nvenc", want: HwAccelNVENC},
			{raw: "rockchip", want: HwAccelRockchip},
			{raw: "auto", wantErr: true},
			{raw: "software", wantErr: true},
		}
		for _, testCase := range cases {
			got, err := ParseHwAccel(testCase.raw)
			if testCase.wantErr {
				if !errors.Is(err, ErrInvalidHwAccel) {
					t.Fatalf("%q: expected ErrInvalidHwAccel, got %v", testCase.raw, err)
				}

				continue
			}
			if err != nil || got != testCase.want {
				t.Fatalf("%q: got %q %v, want %q", testCase.raw, got, err, testCase.want)
			}
		}
	})
}

//nolint:paralleltest // mutates package-level device check hooks
func TestResolvePreferredEncoder_OffAndNVENC(t *testing.T) {
	allure.Test(t, "off and nvenc resolve with software fallback", func(a *allure.Context) {
		t := a.T()
		restoreDeviceChecks(t)

		checkNVIDIAPresent = func() bool { return false }
		encoder, render, reason := resolvePreferredEncoder(HwAccelOff)
		if encoder != EncoderSoftware || render != "" || reason != string(HwAccelOff) {
			t.Fatalf("off: got %s %q %q", encoder, render, reason)
		}

		encoder, _, reason = resolvePreferredEncoder(HwAccelNVENC)
		if encoder != EncoderSoftware || reason != "nvidia device missing" {
			t.Fatalf("nvenc missing: got %s %q", encoder, reason)
		}

		checkNVIDIAPresent = func() bool { return true }
		encoder, _, reason = resolvePreferredEncoder(HwAccelNVENC)
		if encoder != EncoderNVENC || reason != "" {
			t.Fatalf("nvenc present: got %s %q", encoder, reason)
		}
	})
}

//nolint:paralleltest // mutates package-level device check hooks
func TestResolvePreferredEncoder_VAAPIAndQSV(t *testing.T) {
	allure.Test(t, "vaapi and qsv respect intel-only qsv rule", func(a *allure.Context) {
		t := a.T()
		restoreDeviceChecks(t)

		checkRenderDevice = func() string { return testRenderNode }
		checkGPUVendor = func() gpuVendor { return gpuAMD }

		encoder, render, reason := resolvePreferredEncoder(HwAccelVAAPI)
		if encoder != EncoderVAAPI || render != testRenderNode || reason != "" {
			t.Fatalf("vaapi: got %s %q %q", encoder, render, reason)
		}

		encoder, _, reason = resolvePreferredEncoder(HwAccelQSV)
		if encoder != EncoderSoftware || reason != "qsv requires intel gpu" {
			t.Fatalf("qsv on amd: got %s %q", encoder, reason)
		}

		checkGPUVendor = func() gpuVendor { return gpuIntel }
		encoder, render, reason = resolvePreferredEncoder(HwAccelQSV)
		if encoder != EncoderQSV || render != testRenderNode || reason != "" {
			t.Fatalf("qsv on intel: got %s %q %q", encoder, render, reason)
		}
	})
}

func TestBuildArgs_QSVEncoder(t *testing.T) {
	t.Parallel()

	allure.Test(t, "qsv encoder uses qsv hwaccel and scale_qsv", func(a *allure.Context) {
		t := a.T()
		rung := FastStartRung(720)
		args := appendHWGlobalArgs(nil, EncoderQSV, testRenderNode)
		args = appendVideoEncodeArgs(args, EncoderQSV, rung, ToneMapNone, ToneMappingBT2390)
		joined := strings.Join(args, " ")

		if !strings.Contains(joined, "-hwaccel qsv") {
			t.Fatalf("expected qsv hwaccel, got: %s", joined)
		}
		if !strings.Contains(joined, "scale_qsv") || !strings.Contains(joined, string(EncoderQSV)) {
			t.Fatalf("expected scale_qsv and h264_qsv, got: %s", joined)
		}
	})
}

func TestParseDownmixAlgorithm(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parses downmixAlgorithm values", func(a *allure.Context) {
		t := a.T()
		got, err := ParseDownmixAlgorithm("")
		if err != nil || got != DownmixAC4 {
			t.Fatalf("empty default: %q %v", got, err)
		}
		got, err = ParseDownmixAlgorithm("none")
		if err != nil || got != DownmixNone {
			t.Fatalf("none: %q %v", got, err)
		}
		got, err = ParseDownmixAlgorithm("dave750")
		if err != nil || got != DownmixDave750 {
			t.Fatalf("dave750: got %q err=%v", got, err)
		}
	})
}

func TestParseToneMappingAlgorithm(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parses toneMappingAlgorithm values", func(a *allure.Context) {
		t := a.T()
		got, err := ParseToneMappingAlgorithm("")
		if err != nil || got != ToneMappingBT2390 {
			t.Fatalf("empty default: %q %v", got, err)
		}
		got, err = ParseToneMappingAlgorithm("Hable")
		if err != nil || got != ToneMappingHable {
			t.Fatalf("hable: %q %v", got, err)
		}
		_, err = ParseToneMappingAlgorithm("bt2446")
		if !errors.Is(err, ErrInvalidToneMappingAlgorithm) {
			t.Fatalf("expected ErrInvalidToneMappingAlgorithm, got %v", err)
		}
	})
}

func TestNormalizeTranscodeSettings_ToneMapDefaults(t *testing.T) {
	t.Parallel()

	allure.Test(t, "normalize fills tonemap algo; JSON missing enabled defaults true", func(a *allure.Context) {
		t := a.T()
		got, err := NormalizeTranscodeSettings(TranscodeSettings{
			HwAccel:            HwAccelOff,
			ToneMappingEnabled: false,
		})
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if got.ToneMappingAlgorithm != ToneMappingBT2390 {
			t.Fatalf("algo default: %q", got.ToneMappingAlgorithm)
		}
		if got.ToneMappingEnabled {
			t.Fatal("explicit false must stick")
		}

		var decoded TranscodeSettings
		err = json.Unmarshal([]byte(`{"hwAccel":"off","downmixAlgorithm":"ac4"}`), &decoded)
		if err != nil {
			t.Fatalf("unmarshal legacy: %v", err)
		}
		if !decoded.ToneMappingEnabled || decoded.ToneMappingAlgorithm != "" {
			t.Fatalf("legacy decode: %+v", decoded)
		}
		normalized, err := NormalizeTranscodeSettings(decoded)
		if err != nil || !normalized.ToneMappingEnabled ||
			normalized.ToneMappingAlgorithm != ToneMappingBT2390 {
			t.Fatalf("legacy normalize: %+v err=%v", normalized, err)
		}
	})
}

func restoreDeviceChecks(t *testing.T) {
	t.Helper()

	prevNVIDIA := checkNVIDIAPresent
	prevRockchip := checkRockchipPresent
	prevRender := checkRenderDevice
	prevVendor := checkGPUVendor
	t.Cleanup(func() {
		checkNVIDIAPresent = prevNVIDIA
		checkRockchipPresent = prevRockchip
		checkRenderDevice = prevRender
		checkGPUVendor = prevVendor
	})

	checkNVIDIAPresent = func() bool { return false }
	checkRockchipPresent = func() bool { return false }
	checkRenderDevice = func() string { return "" }
	checkGPUVendor = func() gpuVendor { return gpuNone }
}
