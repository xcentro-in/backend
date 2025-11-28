package payment

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
)

// OpsFeeRulesHandler handles ops dashboard fee rules management
type OpsFeeRulesHandler struct {
	feeRulesRepo business_rules.FeeRulesRepository
	logger       *logger.Logger
}

// NewOpsFeeRulesHandler creates a new ops fee rules handler
func NewOpsFeeRulesHandler(
	feeRulesRepo business_rules.FeeRulesRepository,
	logger *logger.Logger,
) *OpsFeeRulesHandler {
	return &OpsFeeRulesHandler{
		feeRulesRepo: feeRulesRepo,
		logger:       logger,
	}
}

// GetAllFeeRules handles GET /ops/v1/fee-rules
func (h *OpsFeeRulesHandler) GetAllFeeRules(c echo.Context) error {
	h.logger.Info("GetAllFeeRules API called (Ops Dashboard)")

	ctx := c.Request().Context()

	// Fetch all active fee rules
	rules, err := h.feeRulesRepo.GetActiveFeeRules(ctx)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch fee rules", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "DATABASE_ERROR",
			"message": "Failed to fetch fee rules",
		})
	}

	// Convert to response format
	response := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		ruleMap := map[string]any{
			"rule_id":         rule.RuleID,
			"payment_method":  rule.PaymentMethod,
			"fee_type":        rule.FeeType,
			"percentage_fee":  rule.PercentageFee,
			"fixed_fee":       rule.FixedFee,
			"gst_percentage":  rule.GSTPercentage,
			"gst_applicable":  rule.GSTApplicable,
			"currency":        rule.Currency,
			"is_active":       rule.IsActive,
		}

		// Add optional card fields if present
		if rule.CardType != "" {
			ruleMap["card_type"] = rule.CardType
		}
		if rule.CardCategory != "" {
			ruleMap["card_category"] = rule.CardCategory
		}

		response = append(response, ruleMap)
	}

	h.logger.InfoWith("Fee rules fetched successfully", "count", len(rules))

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Fee rules retrieved successfully",
		"status":  "success",
		"count":   len(rules),
		"data":    response,
	})
}

// UpdateFeeRuleRequest represents the request body for updating a fee rule
// Note: payment_method, card_type, and card_category are immutable (cannot be updated)
type UpdateFeeRuleRequest struct {
	FeeType        payment.FeeType        `json:"fee_type"`
	PercentageFee  float64                `json:"percentage_fee"`
	FixedFee       int64                  `json:"fixed_fee"`
	GSTPercentage  float64                `json:"gst_percentage"`
	GSTApplicable  bool                   `json:"gst_applicable"`
	IsActive       *bool                  `json:"is_active,omitempty"` // Optional: allows activating/deactivating
}

// UpdateFeeRule handles PUT /ops/v1/fee-rules/:rule_id
func (h *OpsFeeRulesHandler) UpdateFeeRule(c echo.Context) error {
	h.logger.Info("UpdateFeeRule API called (Ops Dashboard)")

	ruleID := c.Param("rule_id")
	if ruleID == "" {
		h.logger.Warn("Missing rule_id parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_RULE_ID",
			"message": "rule_id parameter is required",
		})
	}

	// Parse request body
	var req UpdateFeeRuleRequest
	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Failed to parse request body", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// Validate fee type
	if req.FeeType == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_FEE_TYPE",
			"message": "fee_type is required",
		})
	}

	ctx := c.Request().Context()

	// Build fee rule update (only mutable fields)
	rule := &business_rules.FeeRule{
		FeeType:       req.FeeType,
		PercentageFee: req.PercentageFee,
		FixedFee:      req.FixedFee,
		GSTPercentage: req.GSTPercentage,
		GSTApplicable: req.GSTApplicable,
		IsActive:      true, // Default to active if not specified
	}

	// Handle optional is_active field
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}

	// Update fee rule
	err := h.feeRulesRepo.UpdateFeeRule(ctx, ruleID, rule)
	if err != nil {
		h.logger.ErrorWith("Failed to update fee rule", "error", err, "rule_id", ruleID)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "UPDATE_FAILED",
			"message": "Failed to update fee rule",
		})
	}

	h.logger.InfoWith("Fee rule updated successfully", "rule_id", ruleID)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Fee rule updated successfully",
		"status":  "success",
		"rule_id": ruleID,
	})
}

// DeleteFeeRule handles DELETE /ops/v1/fee-rules/:rule_id
func (h *OpsFeeRulesHandler) DeleteFeeRule(c echo.Context) error {
	h.logger.Info("DeleteFeeRule API called (Ops Dashboard)")

	ruleID := c.Param("rule_id")
	if ruleID == "" {
		h.logger.Warn("Missing rule_id parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_RULE_ID",
			"message": "rule_id parameter is required",
		})
	}

	ctx := c.Request().Context()

	// Deactivate (soft delete) the fee rule
	err := h.feeRulesRepo.DeactivateFeeRule(ctx, ruleID)
	if err != nil {
		h.logger.ErrorWith("Failed to delete fee rule", "error", err, "rule_id", ruleID)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "DELETE_FAILED",
			"message": "Failed to delete fee rule",
		})
	}

	h.logger.InfoWith("Fee rule deleted successfully", "rule_id", ruleID)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Fee rule deleted successfully",
		"status":  "success",
		"rule_id": ruleID,
	})
}

// RegisterOpsFeeRulesRoutes registers ops fee rules routes
func RegisterOpsFeeRulesRoutes(group *echo.Group, handler *OpsFeeRulesHandler) {
	group.GET("/fee-rules", handler.GetAllFeeRules)
	group.PUT("/fee-rules/:rule_id", handler.UpdateFeeRule)
	group.DELETE("/fee-rules/:rule_id", handler.DeleteFeeRule)
}
