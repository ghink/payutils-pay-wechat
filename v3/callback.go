package paywechat

import (
	"net/http"
	"strings"

	"github.com/go-pay/gopay"
	"github.com/go-pay/gopay/wechat/v3"

	"go.gh.ink/payutils/v3/model"
)

// Callback implements model.PayClient. It handles WeChat Pay's async notify.
//
// Payment and refund results arrive on the same route and are distinguished by
// the envelope's event_type, so the refund payload is no longer decrypted with
// the payment struct (which reported an empty trade state and dropped it).
func (c Client) Callback(w http.ResponseWriter, r *http.Request) {
	// Parse notify params
	notifyReq, err := wechat.V3ParseNotify(r)
	if err != nil {
		c.handleErr(r, w, err)
		return
	}

	// Verify sign with the merchant public key map
	certMap := c.Client.WxPublicKeyMap()
	if err = notifyReq.VerifySignByPKMap(certMap); err != nil {
		c.handleErr(r, w, err)
		return
	}

	if strings.HasPrefix(notifyReq.EventType, EventTypeRefundPrefix) {
		c.refundCallback(w, r, notifyReq)
		return
	}

	// Decrypt the cipher text into our struct
	callback := new(WeChatPayCallback)
	if err = notifyReq.DecryptCipherTextToStruct(c.Params.Credential[MerchantAPIv3Key], callback); err != nil {
		c.handleErr(r, w, err)
		return
	}

	// Push status. The framework-provided StatusUpdater trims prefix/suffix and
	// forwards to the user contract (no-op when no contract is configured).
	//
	// The event on the context carries what the signature cannot: WeChat's own
	// order number and the amount of this trade.
	if c.Params.StatusUpdater != nil {
		ctx := model.WithTradeEvent(r.Context(), model.TradeEvent{
			ChannelTxnID: callback.TransactionID,
			Amount:       int64(callback.Amount.Total),
			Currency:     callback.Amount.Currency,
		})

		if err = c.Params.StatusUpdater(
			ctx, r, Name,
			callback.OutTradeNo,
			MapState(callback.TradeState),
			FormatTime(callback.SuccessTime),
		); err != nil {
			c.handleErr(r, w, err)
			return
		}
	}

	c.ack(w)
}

// refundCallback handles a REFUND.* notification.
func (c Client) refundCallback(w http.ResponseWriter, r *http.Request, notifyReq *wechat.V3NotifyReq) {
	callback := new(WeChatRefundCallback)
	if err := notifyReq.DecryptCipherTextToStruct(c.Params.Credential[MerchantAPIv3Key], callback); err != nil {
		c.handleErr(r, w, err)
		return
	}

	// Without a refund-capable contract there is nowhere to push this. Still
	// acknowledge: WeChat would otherwise retry a result nobody is going to read.
	if c.Params.RefundUpdater == nil {
		c.ack(w)
		return
	}

	result := refundResult(
		callback.OutTradeNo, callback.RefundID,
		callback.Status, callback.SuccessTime, callback.Amount,
	)

	if err := c.Params.RefundUpdater(r.Context(), r, Name, callback.OutRefundNo, result); err != nil {
		c.handleErr(r, w, err)
		return
	}

	c.ack(w)
}

// ack acknowledges a notification so WeChat stops resending it.
func (c Client) ack(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if body, mErr := c.Params.Marshal(&wechat.V3NotifyRsp{Code: gopay.SUCCESS, Message: "成功"}); mErr == nil {
		_, _ = w.Write(body)
	}
}

// handleErr routes an error through the user ErrorHandler when present and
// always returns a 500 so WeChat Pay retries the notification.
func (c Client) handleErr(r *http.Request, w http.ResponseWriter, err error) {
	if c.Params.ErrorHandler != nil {
		_ = c.Params.ErrorHandler(r.Context(), r, err)
	}
	w.WriteHeader(http.StatusInternalServerError)
}
