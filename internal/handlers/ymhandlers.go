package handlers

import (
	"log"
	"strconv"

	"github.com/rvinnie/yookassa-sdk-go/yookassa"
	yoocommon "github.com/rvinnie/yookassa-sdk-go/yookassa/common"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
)

func (h *Handler) CreatePayment(amount int, productname string) *yoopayment.Payment {
	amountStr := strconv.Itoa(amount) + ".00"

	yooclient := yookassa.NewClient(h.YouKassaID, h.YouKassaSecretKey)
	paymentHandler := yookassa.NewPaymentHandler(yooclient)

	payment, _ := paymentHandler.CreatePayment(&yoopayment.Payment{
		Amount: &yoocommon.Amount{
			Value:    amountStr,
			Currency: "RUB",
		},
		Confirmation: yoopayment.Redirect{
			Type:      "redirect",
			ReturnURL: h.YouKassaReturnURL,
		},
		Description: productname,
		Capture:     true,
	})

	log.Printf("Payment created: %+v", payment)

	return payment
}

// CancelPayment отменяет платёж по ID в ЮKassa.
func (h *Handler) CancelPayment(paymentID string) error {
	yooclient := yookassa.NewClient(h.YouKassaID, h.YouKassaSecretKey)
	paymentHandler := yookassa.NewPaymentHandler(yooclient)
	_, err := paymentHandler.CancelPayment(paymentID)
	if err != nil {
		log.Printf("CancelPayment %s: %v", paymentID, err)
		return err
	}
	return nil
}
