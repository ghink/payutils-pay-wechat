package paywechat

import (
	"testing"
	"time"

	"github.com/go-pay/gopay/wechat/v3"

	"go.gh.ink/payutils/v3/model"
)

// The driver must offer refund lookup, otherwise payutils reports the capability
// as missing instead of letting callers poll.
var (
	_ model.PayClient     = Client{}
	_ model.RefundQuerier = Client{}
)

func TestRefundResultMapsUpstreamFields(t *testing.T) {
	got := refundResult(
		"pre_T123_suf", "50300a", RefundStateSuccess, "2024-01-02T15:04:05+08:00",
		&wechat.RefundOrderAmount{Total: 3000, Refund: 1000, Currency: "CNY"},
	)

	if got.State != model.RefundStateSuccess {
		t.Errorf("state = %q, want SUCCESS", got.State)
	}
	if got.TradeID != "pre_T123_suf" {
		t.Errorf("trade id should come back as the upstream reported it, got %q", got.TradeID)
	}
	if got.ChannelRefundID != "50300a" {
		t.Errorf("channelRefundID = %q", got.ChannelRefundID)
	}
	if got.RefundAmount != 1000 || got.Currency != "CNY" {
		t.Errorf("amount = %d %q, want 1000 CNY", got.RefundAmount, got.Currency)
	}
	if want := time.Date(2024, 1, 2, 7, 4, 5, 0, time.UTC); !got.SuccessTime.Equal(want) {
		t.Errorf("successTime = %v, want %v", got.SuccessTime, want)
	}
}

func TestRefundResultWithoutAmount(t *testing.T) {
	// A missing amount block means "the upstream did not report one", which must
	// not be folded into "refunded zero".
	got := refundResult("T1", "R1", RefundStateProcessing, "", nil)

	if got.State != model.RefundStateProcessing {
		t.Errorf("state = %q, want PROCESSING", got.State)
	}
	if got.RefundAmount != 0 || got.Currency != "" {
		t.Errorf("amount should stay unset, got %d %q", got.RefundAmount, got.Currency)
	}
	if !got.SuccessTime.IsZero() {
		t.Errorf("successTime should stay zero, got %v", got.SuccessTime)
	}
}

func TestRefundResultUnknownStateIsNotSuccess(t *testing.T) {
	// A state WeChat adds later must never be read as a settled refund.
	got := refundResult("T1", "R1", "SOMETHING_NEW", "", &wechat.RefundOrderAmount{Refund: 100})

	if got.State != model.RefundStateUnknown {
		t.Fatalf("state = %q, want UNKNOWN", got.State)
	}
	if got.RefundAmount != 100 {
		t.Errorf("amount should still be reported, got %d", got.RefundAmount)
	}
}
