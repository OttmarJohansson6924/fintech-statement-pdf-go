package statement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"example.com/fintech-statement/internal/infrai"
)

type Provider interface {
	UsageTimeseries(context.Context) (infrai.UsageSeries, error)
	GeneratePDF(context.Context, string, string) (infrai.PDFResult, error)
}

type Service struct {
	provider Provider
	now      func() time.Time
}

type Request struct {
	AccountID string         `json:"account_id"`
	Period    Period         `json:"period"`
	Payments  []PaymentEvent `json:"payments"`
}

type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type PaymentEvent struct {
	ID       string `json:"id"`
	PostedAt string `json:"posted_at"`
	Kind     string `json:"kind"`
	Amount   int64  `json:"amount_minor"`
	Currency string `json:"currency"`
	Risk     string `json:"risk"`
}

type AuditNotification struct {
	Type      string `json:"type"`
	AccountID string `json:"account_id"`
	EventID   string `json:"event_id,omitempty"`
	At        string `json:"at"`
	Message   string `json:"message"`
}

type Response struct {
	Status        string              `json:"status"`
	PDF           *infrai.PDFResult   `json:"pdf,omitempty"`
	Notifications []AuditNotification `json:"notifications"`
}

func NewService(provider Provider, now func() time.Time) *Service {
	return &Service{provider: provider, now: now}
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input Request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid statement request"})
		return
	}

	result, err := s.Export(r.Context(), input)
	if err != nil {
		var apiErr *infrai.APIError
		if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
			writeJSON(w, apiErr.Status, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "statement provider request failed"})
		return
	}
	status := http.StatusCreated
	if result.Status == "review_required" {
		status = http.StatusAccepted
	}
	writeJSON(w, status, result)
}

func (s *Service) Export(ctx context.Context, input Request) (Response, error) {
	if input.AccountID == "" || input.Period.Start == "" || input.Period.End == "" {
		return Response{}, &infrai.APIError{Status: http.StatusBadRequest, Code: "INVALID_STATEMENT", Message: "account_id and period are required"}
	}

	now := s.now().UTC().Format(time.RFC3339)
	for _, payment := range input.Payments {
		if payment.Risk == "high" {
			return Response{
				Status: "review_required",
				Notifications: []AuditNotification{{
					Type: "statement.review_required", AccountID: input.AccountID, EventID: payment.ID,
					At: now, Message: "high-risk payment held for review before statement export",
				}},
			}, nil
		}
	}

	usage, err := s.provider.UsageTimeseries(ctx)
	if err != nil {
		return Response{}, err
	}
	markdown := renderStatement(input, usage)
	idempotencyKey := statementKey(input)
	pdf, err := s.provider.GeneratePDF(ctx, markdown, idempotencyKey)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Status: "exported",
		PDF:    &pdf,
		Notifications: []AuditNotification{{
			Type: "statement.exported", AccountID: input.AccountID, At: now,
			Message: "statement PDF generated from ordered payment and usage records",
		}},
	}, nil
}

func renderStatement(input Request, usage infrai.UsageSeries) string {
	payments := append([]PaymentEvent(nil), input.Payments...)
	sort.Slice(payments, func(i, j int) bool {
		if payments[i].PostedAt == payments[j].PostedAt {
			return payments[i].ID < payments[j].ID
		}
		return payments[i].PostedAt < payments[j].PostedAt
	})

	var b strings.Builder
	fmt.Fprintf(&b, "# Account statement\n\nAccount: `%s`  \nPeriod: %s to %s\n\n", input.AccountID, input.Period.Start, input.Period.End)
	b.WriteString("## Payment events\n\n| Posted | Event | Kind | Amount |\n|---|---|---|---:|\n")
	for _, payment := range payments {
		fmt.Fprintf(&b, "| %s | %s | %s | %.2f %s |\n", payment.PostedAt, payment.ID, payment.Kind, float64(payment.Amount)/100, payment.Currency)
	}
	b.WriteString("\n## Metered usage\n\n| Timestamp | Quantity | Cost USD |\n|---|---:|---:|\n")
	for _, point := range usage.Points {
		fmt.Fprintf(&b, "| %s | %.3f | %.4f |\n", point.Timestamp, point.Quantity, point.CostUSD)
	}
	return b.String()
}

func statementKey(input Request) string {
	hash := sha256.Sum256([]byte(input.AccountID + "\x00" + input.Period.Start + "\x00" + input.Period.End))
	return "statement-" + hex.EncodeToString(hash[:16])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
