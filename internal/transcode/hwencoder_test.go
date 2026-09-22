package transcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestPCIIDToVendor(t *testing.T) {
	t.Parallel()

	allure.Test(t, "maps PCI vendor IDs to GPU vendors", func(a *allure.Context) {
		t := a.T()
		cases := []struct {
			id   string
			want gpuVendor
		}{
			{id: "0x8086", want: gpuIntel},
			{id: "0x10DE", want: gpuNVIDIA},
			{id: "0x1002", want: gpuAMD},
			{id: "", want: gpuNone},
			{id: "0x1234", want: gpuUnknown},
		}

		for _, tc := range cases {
			if got := pciIDToVendor(tc.id); got != tc.want {
				t.Fatalf("pciIDToVendor(%q) = %q, want %q", tc.id, got, tc.want)
			}
		}
	})
}

func TestBuildFaststartHLSArgs_VAAPIEncoder(t *testing.T) {
	t.Parallel()

	allure.Test(t, "vaapi encoder initializes vaapi device", func(a *allure.Context) {
		t := a.T()
		rung := FastStartRung(720)
		args := appendHWGlobalArgs(nil, EncoderVAAPI, "/dev/dri/renderD128")
		args = appendVideoEncodeArgs(args, EncoderVAAPI, rung, ToneMapNone, ToneMappingBT2390)
		joined := strings.Join(args, " ")

		if !strings.Contains(joined, "-init_hw_device") || !strings.Contains(joined, "vaapi=va:") {
			t.Fatalf("expected vaapi init, got: %s", joined)
		}
		if !strings.Contains(joined, "-c:v h264_vaapi") {
			t.Fatalf("expected h264_vaapi, got: %s", joined)
		}
	})
}

func TestHWEncoder_DeviceHelpers(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"device helpers tolerate missing and present sysfs files",
		func(a *allure.Context) {
			t := a.T()
			_ = nvidiaDevicePresent()
			_ = discoverRenderDevice()
			_ = primaryGPUVendor()
			_ = firstRenderDevice()

			missing := readPCIVendor(filepath.Join(t.TempDir(), "missing-vendor"))
			if missing != gpuNone {
				t.Fatalf("missing vendor file: got %q", missing)
			}

			vendorPath := filepath.Join(t.TempDir(), "vendor")
			err := os.WriteFile(vendorPath, []byte("0x8086\n"), 0o600)
			if err != nil {
				t.Fatalf("write vendor: %v", err)
			}
			if got := readPCIVendor(vendorPath); got != gpuIntel {
				t.Fatalf("readPCIVendor: got %q want intel", got)
			}
		},
	)
}

//nolint:paralleltest // mutates package-level device check hooks
func TestResolveVideoEncoder_LogsFallback(t *testing.T) {
	allure.Test(
		t,
		"ResolveVideoEncoder falls back when preferred devices are missing",
		func(a *allure.Context) {
			t := a.T()
			restoreDeviceChecks(t)

			encoder, render := ResolveVideoEncoder(HwAccel("bogus"))
			if encoder != EncoderSoftware || render != "" {
				t.Fatalf("bogus pref: got %s %q", encoder, render)
			}

			checkNVIDIAPresent = func() bool { return false }
			encoder, _ = ResolveVideoEncoder(HwAccelNVENC)
			if encoder != EncoderSoftware {
				t.Fatalf("nvenc missing: got %s", encoder)
			}

			checkRenderDevice = func() string { return "" }
			encoder, _ = ResolveVideoEncoder(HwAccelVAAPI)
			if encoder != EncoderSoftware {
				t.Fatalf("vaapi missing render: got %s", encoder)
			}
		},
	)
}

//nolint:paralleltest // mutates package-level device check hooks
func TestResolvePreferredEncoder_Rockchip(t *testing.T) {
	allure.Test(t, "rockchip resolves from mpp_service", func(a *allure.Context) {
		t := a.T()
		restoreDeviceChecks(t)

		encoder, _, reason := resolvePreferredEncoder(HwAccelRockchip)
		if encoder != EncoderSoftware || reason != "mpp_service missing" {
			t.Fatalf("missing mpp: got %s %q", encoder, reason)
		}

		checkRockchipPresent = func() bool { return true }
		encoder, _, reason = resolvePreferredEncoder(HwAccelRockchip)
		if encoder != EncoderRockchip || reason != "" {
			t.Fatalf("present mpp: got %s %q", encoder, reason)
		}
	})
}
