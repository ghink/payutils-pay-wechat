package paywechat

import (
	"context"
	"strings"

	"github.com/go-pay/gopay"
	"github.com/go-pay/gopay/wechat/v3"

	"go.gh.ink/payutils/v3/errors"
	"go.gh.ink/payutils/v3/model"
	"go.gh.ink/payutils/v3/router"
)

func (c Client) Status(tradeID string) (model.ReturnStatus, error) {
	outTradeNo := strings.Join([]string{c.Params.TradeIDPrefix, tradeID, c.Params.TradeIDSuffix}, "")

	// Query order by out_trade_no (idType = 2)
	wxRsp, err := c.Client.V3TransactionQueryOrder(context.Background(), 2, outTradeNo)
	if err != nil {
		return model.ReturnStatus{}, err
	}

	// Check return status (404 means trade not exist)
	if wxRsp.Code != 0 && wxRsp.Code != 404 {
		return model.ReturnStatus{}, ErrWeChatPayRespCodeInvalid.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}
	if wxRsp.Code == 404 {
		return model.ReturnStatus{}, errors.ErrTradeNotExist.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}

	return model.ReturnStatus{
		TradeStatus: MapState(wxRsp.Response.TradeState),
		Upstream:    Name,
		Time:        FormatTime(wxRsp.Response.SuccessTime),
		// WeChat's own order number, only present once the trade actually paid.
		ChannelTxnID: wxRsp.Response.TransactionId,
	}, nil
}

func (c Client) Close(tradeID string) error {
	outTradeNo := strings.Join([]string{c.Params.TradeIDPrefix, tradeID, c.Params.TradeIDSuffix}, "")

	wxRsp, err := c.Client.V3TransactionCloseOrder(context.Background(), outTradeNo)
	if err != nil {
		return err
	}

	if wxRsp.Code != 0 && wxRsp.Code != 404 {
		return ErrWeChatPayRespCodeInvalid.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}
	if wxRsp.Code == 404 {
		return errors.ErrTradeNotExist.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}

	return nil
}

// Refund issues a refund. WeChat accepts the request and settles it
// asynchronously, so a nil error only means the refund was taken: the reported
// State is what tells whether the money has actually moved.
func (c Client) Refund(
	tradeID string, curr string, refundID string, totalAmount int64, refundAmount int64, reason string,
) (model.RefundResult, error) {
	outTradeNo := strings.Join([]string{c.Params.TradeIDPrefix, tradeID, c.Params.TradeIDSuffix}, "")
	outRefundNo := strings.Join([]string{c.Params.TradeIDPrefix, refundID, c.Params.TradeIDSuffix}, "")
	notifyURL := router.Notify(c.Params.Endpoint, Name)

	bm := make(gopay.BodyMap)
	bm.Set("out_trade_no", outTradeNo).
		Set("out_refund_no", outRefundNo).
		Set("reason", reason).
		Set("notify_url", notifyURL).
		SetBodyMap("amount", func(bm gopay.BodyMap) {
			bm.Set("total", totalAmount).
				Set("refund", refundAmount).
				Set("currency", curr)
		})

	wxRsp, err := c.Client.V3Refund(context.Background(), bm)
	if err != nil {
		return model.RefundResult{}, err
	}

	if wxRsp.Code != 0 && wxRsp.Code != 404 {
		return model.RefundResult{}, ErrWeChatPayRespCodeInvalid.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}
	if wxRsp.Code == 404 {
		return model.RefundResult{}, errors.ErrTradeNotExist.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}

	return refundResult(
		wxRsp.Response.OutTradeNo, wxRsp.Response.RefundId,
		wxRsp.Response.Status, wxRsp.Response.SuccessTime, wxRsp.Response.Amount,
	), nil
}

// QueryRefund looks a refund up by our own refund number.
func (c Client) QueryRefund(tradeID string, refundID string) (model.RefundResult, error) {
	outRefundNo := strings.Join([]string{c.Params.TradeIDPrefix, refundID, c.Params.TradeIDSuffix}, "")

	wxRsp, err := c.Client.V3RefundQuery(context.Background(), outRefundNo, nil)
	if err != nil {
		return model.RefundResult{}, err
	}

	if wxRsp.Code != 0 && wxRsp.Code != 404 {
		return model.RefundResult{}, ErrWeChatPayRespCodeInvalid.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}
	if wxRsp.Code == 404 {
		return model.RefundResult{}, errors.ErrRefundNotExist.
			WithUpstreamName(Name).
			WithUpstreamCode(wxRsp.ErrResponse.Code).
			WithUpstreamMessage(wxRsp.ErrResponse.Message).
			WithUpstreamResponse(wxRsp)
	}

	// tradeID is not echoed by this endpoint; the refund number is what identifies
	// the refund unambiguously, so the caller-side trade id comes back as reported.
	return refundResult(
		wxRsp.Response.OutTradeNo, wxRsp.Response.RefundId,
		wxRsp.Response.Status, wxRsp.Response.SuccessTime, wxRsp.Response.Amount,
	), nil
}

// refundResult folds a WeChat refund payload into the framework result. The
// refund and the refund-query responses carry the same fields.
func refundResult(
	outTradeNo, channelRefundID, status, successTime string, amount *wechat.RefundOrderAmount,
) model.RefundResult {
	result := model.RefundResult{
		State:           MapRefundState(status),
		TradeID:         outTradeNo,
		ChannelRefundID: channelRefundID,
		SuccessTime:     FormatTime(successTime),
	}

	// A nil amount means WeChat did not report one, which is different from a
	// refund of zero; leave the amount empty rather than inventing one.
	if amount != nil {
		result.RefundAmount = int64(amount.Refund)
		result.Currency = amount.Currency
	}

	return result
}
