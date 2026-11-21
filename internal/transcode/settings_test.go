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

	for _, tc := range []struct {
		name    string
		raw     string
		want    HwAccel
		wantErr bool
	}{
		{name: "off", raw: string(HwAccelOff), want: HwAccelOff},
		{name: "empty", raw: "", want: HwAccelOff},
		{name: "QSV upper", raw: "QSV", want: HwAccelQSV},
		{name: "vaapi", raw: "vaapi", want: HwAccelVAAPI},
		{name: "nvenc", raw: "nvenc", want: HwAccelNVENC},
		{name: "rockchip", raw: "rockchip", want: HwAccelRockchip},
		{name: "auto rejected", raw: "auto", wantErr: true},
		{name: "software rejected", raw: "software", wantErr: true},
	} {
		allure.Test(t, "parse hwAccel — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got, err := ParseHwAccel(tc.raw)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidHwAccel) {
					t.Fatalf("%q: expected ErrInvalidHwAccel, got %v", tc.raw, err)
				}

				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("%q: got %q %v, want %q", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestResolvePreferredEncoder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accel      HwAccel
		setup      func()
		wantEnc    VideoEncoder
		wantRender string
		wantReason string
	}{
		{
			name:       "off uses software",
			accel:      HwAccelOff,
			setup:      func() { checkNVIDIAPresent = func() bool { return false } },
			wantEnc:    EncoderSoftware,
			wantReason: string(HwAccelOff),
		},
		{
			name:       "nvenc missing device",
			accel:      HwAccelNVENC,
			setup:      func() { checkNVIDIAPresent = func() bool { return false } },
			wantEnc:    EncoderSoftware,
			wantReason: "nvidia device missing",
		},
		{
			name:    "nvenc present",
			accel:   HwAccelNVENC,
			setup:   func() { checkNVIDIAPresent = func() bool { return true } },
			wantEnc: EncoderNVENC,
		},
		{
			name:  "vaapi with render node",
			accel: HwAccelVAAPI,
			setup: func() {
				checkRenderDevice = func() string { return testRenderNode }
				checkGPUVendor = func() gpuVendor { return gpuAMD }
			},
			wantEnc:    EncoderVAAPI,
			wantRender: testRenderNode,
		},
		{
			name:  "qsv rejected on amd",
			accel: HwAccelQSV,
			setup: func() {
				checkRenderDevice = func() string { return testRenderNode }
				checkGPUVendor = func() gpuVendor { return gpuAMD }
			},
			wantEnc:    EncoderSoftware,
			wantRender: testRenderNode,
			wantReason: "qsv requires intel gpu",
		},
		{
			name:  "qsv on intel",
			accel: HwAccelQSV,
			setup: func() {
				checkRenderDevice = func() string { return testRenderNode }
				checkGPUVendor = func() gpuVendor { return gpuIntel }
			},
			wantEnc:    EncoderQSV,
			wantRender: testRenderNode,
		},
	} {
		allure.Test(t, "resolve preferred encoder — "+tc.name, func(a *allure.Context) {
			t := a.T()
			restoreDeviceChecks(t)
			tc.setup()
			encoder, render, reason := resolvePreferredEncoder(tc.accel)
			if encoder != tc.wantEnc || render != tc.wantRender || reason != tc.wantReason {
				t.Fatalf("got %s %q %q want %s %q %q",
					encoder, render, reason, tc.wantEnc, tc.wantRender, tc.wantReason)
			}
		})
	}
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

	for _, tc := range []struct {
		name    string
		raw     string
		want    DownmixAlgorithm
		wantErr bool
	}{
		{name: "empty default ac4", raw: "", want: DownmixAC4},
		{name: "none", raw: "none", want: DownmixNone},
		{name: "dave750", raw: "dave750", want: DownmixDave750},
		{name: "ac4", raw: "ac4", want: DownmixAC4},
		{name: "nightmode", raw: "nightmodeDialogue", want: DownmixNightmodeDialogue},
		{name: "rfc7845", raw: "rfc7845", want: DownmixRFC7845},
		{name: "bogus", raw: "bogus", wantErr: true},
	} {
		allure.Test(t, "parse downmix algorithm — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got, err := ParseDownmixAlgorithm(tc.raw)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidDownmix) {
					t.Fatalf("expected ErrInvalidDownmix, got %v", err)
				}

				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v want %q", got, err, tc.want)
			}
		})
	}
}

func TestParseDownmixBoost(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		boost   float64
		algo    DownmixAlgorithm
		want    float64
		wantErr bool
	}{
		{name: "none default 2", boost: 0, algo: DownmixNone, want: 2},
		{name: "ac4 default 1", boost: 0, algo: DownmixAC4, want: 1},
		{name: "explicit", boost: 1.25, algo: DownmixAC4, want: 1.25},
		{name: "too low", boost: 0.4, algo: DownmixAC4, wantErr: true},
	} {
		allure.Test(t, "parse downmix boost — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got, err := ParseDownmixBoost(tc.boost, tc.algo)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidDownmixBoost) {
					t.Fatalf("expected ErrInvalidDownmixBoost, got %v", err)
				}

				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %v err=%v want %v", got, err, tc.want)
			}
		})
	}
}

func TestParseToneMappingAlgorithm(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		raw     string
		want    ToneMappingAlgorithm
		wantErr bool
	}{
		{name: "empty default bt2390", raw: "", want: ToneMappingBT2390},
		{name: "Hable", raw: "Hable", want: ToneMappingHable},
		{name: "bt2446 rejected", raw: "bt2446", wantErr: true},
	} {
		allure.Test(t, "parse tone mapping — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got, err := ParseToneMappingAlgorithm(tc.raw)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidToneMappingAlgorithm) {
					t.Fatalf("expected ErrInvalidToneMappingAlgorithm, got %v", err)
				}

				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v want %q", got, err, tc.want)
			}
		})
	}
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
