package business

import (
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/shared/logger"
)

type BusinessHandler struct {
	businessService business.BusinessService
	logger          *logger.Logger
}

func NewBusinessHandler(businessService business.BusinessService) *BusinessHandler {
	return &BusinessHandler{
		businessService: businessService,
		logger:          logger.GetDefault(),
	}
}
