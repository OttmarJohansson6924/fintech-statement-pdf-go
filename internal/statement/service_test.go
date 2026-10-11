package statement

import (
	"context"
	"strings"
	"testing"
	"time"

	"example.com/fintech-statement/internal/infrai"
)

type fakeProvider struct {
	usageCalls int
	pdfCalls   int
	markdown   string
}

func (f *fakeProvider) UsageTimeseries(context.Context) (infrai.UsageSeries, error) {
	f.usageCalls++
	return infrai.UsageSeries{Points: []infrai.UsagePoint{{Timestamp: "2026-08-31", Quantity: 42, CostUSD: 1.25}}}, nil
}

func (f *fakeProvider) GeneratePDF(_ context.Context, markdown, _ string) (infrai.PDFResult, error) {
	f.pdfCalls++
	f.markdown = markdown
	return infrai.PDFResult{JobID: "job_123"}, nil
}

func TestExportDecision(t *testing.T) {
	fixed := func() time.Time { return time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC) }
	tests := []struct {
		name          string
		risk          string
		wantStatus    string
		wantUsageCall int
		wantPDFCall   int
		wantNotice    string
	}{
		{name: "normal payment exports", risk: "low", wantStatus: "exported", wantUsageCall: 1, wantPDFCall: 1, wantNotice: "statement.exported"},
		{name: "high risk payment waits for review", risk: "high", wantStatus: "review_required", wantUsageCall: 0, wantPDFCall: 0, wantNotice: "statement.review_required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeProvider{}
			service := NewService(provider, fixed)
			got, err := service.Export(context.Background(), Request{
				AccountID: "acct_ledger_7",
				Period:    Period{Start: "2026-08-01", End: "2026-08-31"},
				Payments:  []PaymentEvent{{ID: "pay_9", PostedAt: "2026-08-20", Kind: "card", Amount: 1299, Currency: "USD", Risk: tt.risk}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tt.wantStatus || provider.usageCalls != tt.wantUsageCall || provider.pdfCalls != tt.wantPDFCall {
				t.Fatalf("status=%q usage_calls=%d pdf_calls=%d", got.Status, provider.usageCalls, provider.pdfCalls)
			}
			if len(got.Notifications) != 1 || got.Notifications[0].Type != tt.wantNotice {
				t.Fatalf("notifications=%+v", got.Notifications)
			}
			if tt.wantPDFCall == 1 && !strings.Contains(provider.markdown, "42.000") {
				t.Fatalf("metered usage missing from statement: %s", provider.markdown)
			}
		})
	}
}
