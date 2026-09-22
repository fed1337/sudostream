package transcode

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestRemuxEligible_BlocksHDR(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"RemuxEligible rejects HDR H.264 even when codec is remuxable",
		func(a *allure.Context) {
			t := a.T()
			sdr := SourceInfo{VideoStreamCount: 1, VideoCodec: codecH264}
			if !RemuxEligible(sdr) {
				t.Fatal("SDR h264 should remux")
			}

			hdr := SourceInfo{
				VideoStreamCount: 1,
				VideoCodec:       codecH264,
				ColorTransfer:    colorTransferPQ,
			}
			if RemuxEligible(hdr) {
				t.Fatal("HDR h264 must not remux")
			}
		},
	)
}
