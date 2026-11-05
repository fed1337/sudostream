package trash

import (
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestService_StartAndShutdownAutoPurge(t *testing.T) {
	t.Parallel()

	allure.Test(t, "StartAutoPurge and Shutdown are idempotent", func(a *allure.Context) {
		_ = a.T()
		svc := NewService(nil, nil, nil, nil)
		svc.StartAutoPurge()
		svc.StartAutoPurge()
		svc.Shutdown()
		svc.Shutdown()
	})
}

func TestNextUTCMidnight(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "midday advances to next day",
			now:  time.Date(2026, 11, 5, 12, 30, 0, 0, time.UTC),
			want: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "exact midnight advances a full day",
			now:  time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "non-UTC input normalizes",
			now:  time.Date(2026, 11, 5, 23, 0, 0, 0, time.FixedZone("CET", 2*3600)),
			want: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1),
		},
	} {
		allure.Test(t, "nextUTCMidnight — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got := nextUTCMidnight(tc.now)
			if !got.Equal(tc.want) {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
