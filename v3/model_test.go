package paywechat

import (
	"testing"
	"time"

	"go.gh.ink/payutils/v3/model"
)

func TestMapState(t *testing.T) {
	cases := []struct {
		in   string
		want model.TradeState
	}{
		{TradeStateNotPay, model.TradeStatePending},
		{TradeStateSuccess, model.TradeStateSuccess},
		{TradeStateRefund, model.TradeStateClosed},
		{TradeStateClosed, model.TradeStateClosed},
		{"PAYERROR", model.TradeStateUnknown},
		{"", model.TradeStateUnknown},
	}
	for _, c := range cases {
		if got := MapState(c.in); got != c.want {
			t.Errorf("MapState(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	if got := FormatTime(""); !got.IsZero() {
		t.Errorf("FormatTime(\"\") = %v, want zero", got)
	}
	if got := FormatTime("not-a-time"); !got.IsZero() {
		t.Errorf("FormatTime(invalid) = %v, want zero", got)
	}

	// WeChat uses RFC3339.
	got := FormatTime("2024-01-02T15:04:05+08:00")
	if got.IsZero() {
		t.Fatal("FormatTime(valid) returned zero")
	}
	wantUTC := time.Date(2024, 1, 2, 7, 4, 5, 0, time.UTC)
	if !got.Equal(wantUTC) {
		t.Errorf("FormatTime = %v (UTC %v), want %v", got, got.UTC(), wantUTC)
	}
}

func TestMapRefundState(t *testing.T) {
	cases := []struct {
		in   string
		want model.RefundState
	}{
		{RefundStateSuccess, model.RefundStateSuccess},
		{RefundStateProcessing, model.RefundStateProcessing},
		{RefundStateClosed, model.RefundStateClosed},
		{RefundStateAbnormal, model.RefundStateAbnormal},
		{"SOMETHING_ELSE", model.RefundStateUnknown},
		{"", model.RefundStateUnknown},
	}
	for _, c := range cases {
		if got := MapRefundState(c.in); got != c.want {
			t.Errorf("MapRefundState(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
