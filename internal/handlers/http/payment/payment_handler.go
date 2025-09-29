package payment

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// PaymentHandler handles payment-related HTTP requests
type PaymentHandler struct {
	// No dependencies yet - just basic handlers with print statements
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{}
}

// FetchWallet handles POST /process_payment/fetch_wallet
func (h *PaymentHandler) FetchWallet(c echo.Context) error {
	fmt.Println("=== FETCH_WALLET API CALLED ===")

	// Print request body
	var body map[string]any
	if err := c.Bind(&body); err != nil {
		fmt.Printf("Error binding request body: %v\n", err)
	} else {
		fmt.Printf("Request Body: %+v\n", body)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "fetch_wallet endpoint called",
		"endpoint": "POST /v1/process_payment/fetch_wallet",
		"status": "success",
		"received_data": body,
	})
}

// GetFeeRules handles POST /process_payment/get_fee_rules
func (h *PaymentHandler) GetFeeRules(c echo.Context) error {
	fmt.Println("=== GET_FEE_RULES API CALLED ===")

	// Print request body
	var body map[string]any
	if err := c.Bind(&body); err != nil {
		fmt.Printf("Error binding request body: %v\n", err)
	} else {
		fmt.Printf("Request Body: %+v\n", body)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "get_fee_rules endpoint called",
		"endpoint": "POST /v1/process_payment/get_fee_rules",
		"status": "success",
		"received_data": body,
	})
}

// Tokenize handles POST /process_payment/tokenize
func (h *PaymentHandler) Tokenize(c echo.Context) error {
	fmt.Println("=== TOKENIZE API CALLED ===")

	// Print request body
	var body map[string]any
	if err := c.Bind(&body); err != nil {
		fmt.Printf("Error binding request body: %v\n", err)
	} else {
		fmt.Printf("Request Body: %+v\n", body)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "tokenize endpoint called",
		"endpoint": "POST /v1/process_payment/tokenize",
		"status": "success",
		"received_data": body,
	})
}

// MakePayment handles POST /process_payment/make_payment
func (h *PaymentHandler) MakePayment(c echo.Context) error {
	fmt.Println("=== MAKE_PAYMENT API CALLED ===")

	// Print request body
	var body map[string]any
	if err := c.Bind(&body); err != nil {
		fmt.Printf("Error binding request body: %v\n", err)
	} else {
		fmt.Printf("Request Body: %+v\n", body)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "make_payment endpoint called",
		"endpoint": "POST /v1/process_payment/make_payment",
		"status": "success",
		"received_data": body,
	})
}

// RegisterPaymentRoutes registers all payment routes
func RegisterPaymentRoutes(group *echo.Group) {
	handler := NewPaymentHandler()

	group.POST("/fetch_wallet", handler.FetchWallet)
	group.POST("/get_fee_rules", handler.GetFeeRules)
	group.POST("/tokenize", handler.Tokenize)
	group.POST("/make_payment", handler.MakePayment)
}