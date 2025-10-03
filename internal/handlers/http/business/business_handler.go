package business

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/providers/fileupload"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

type BusinessHandler struct {
	businessService business.BusinessService
	authService     auth.AuthService
	S3Config        *config.S3Config
	logger          *logger.Logger
}

func NewBusinessHandler(businessService business.BusinessService, authService auth.AuthService, s3Config *config.S3Config) *BusinessHandler {
	return &BusinessHandler{
		businessService: businessService,
		authService:     authService,
		S3Config:        s3Config,
		logger:          logger.GetDefault(),
	}
}

func RegisterBusinessRoutes(e *echo.Group, businessService business.BusinessService, authService auth.AuthService, s3Config *config.S3Config) {
	handler := NewBusinessHandler(businessService, authService, s3Config)

	businessGroup := e.Group("/business")
	// Apply middleware to the business group, not the parent group
	businessGroup.Use(authService.GetEchoAuthMiddleware())
	businessGroup.Use(businessService.GetUserRolesMiddleware())

	businessGroup.POST("", handler.CreateBusiness)
	businessGroup.GET("/roles", handler.GetUserRoles)
	businessGroup.GET("/kyb", handler.GetKYBStatus)
	businessGroup.POST("/kyb", handler.UpdateBusinessKYBData)
	businessGroup.GET("/kyb/upload", handler.UploadKYBDocuments)
	businessGroup.POST("/kyb/confirm-upload", handler.ConfirmKYBDocumentsUpload)
}

func (h *BusinessHandler) CreateBusiness(c echo.Context) error {
	var CreateBusinessDTO business.CreateBusinessDTO

	ur := c.Get("userRoles")
	if ur != nil {
		return c.JSON(403, map[string]interface{}{"message": "User is already associated with a business", "data": ur})
	}

	if err := c.Bind(&CreateBusinessDTO); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	userIdRaw := c.Get("userId")
	if userIdRaw == nil {
		return c.JSON(400, map[string]string{"message": "Unauthorised request"})
	}
	userId := userIdRaw.(string)

	bus, err := h.businessService.CreateBusinessByNameAndGSTIN(c.Request().Context(), userId, &CreateBusinessDTO)
	if err != nil {
		h.logger.Errorf("Error creating business: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to create business"})
	}

	return c.JSON(201, map[string]interface{}{"message": "Business created successfully", "data": bus})
}

func (h *BusinessHandler) UpdateBusinessKYBData(c echo.Context) error {
	var UpdateBusinessKYBDTO business.BusinessKYBDTO

	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}

	roles := ur.([]*business.BusinessUserRelation)
	if len(roles) == 0 {
		return c.JSON(403, map[string]interface{}{"message": "User is not associated with any business", "data": nil})
	}

	if err := c.Bind(&UpdateBusinessKYBDTO); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	if !(len(roles) == 1 && roles[0].KybInitiated && roles[0].BID == UpdateBusinessKYBDTO.BID) {
		return c.JSON(403, map[string]interface{}{"message": "User is unauthorised to update the business kyb", "data": nil})
	}

	if err := h.businessService.UpdateBusinessKYBData(c.Request().Context(), &UpdateBusinessKYBDTO); err != nil {
		h.logger.Errorf("Error updating business KYB data: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to update business KYB data"})
	}

	return c.JSON(200, map[string]interface{}{"message": "Business KYB data updated successfully", "data": nil})
}

func (h *BusinessHandler) GetUserRoles(c echo.Context) error {
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)
	return c.JSON(200, map[string]interface{}{"message": "Fetched user roles successfully", "data": roles})
}

func (h *BusinessHandler) GetKYBStatus(c echo.Context) error {
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)

	userIdRaw := c.Get("userId")
	if userIdRaw == nil {
		return c.JSON(400, map[string]interface{}{"message": "Unauthorised request", "data": nil})
	}
	userId := userIdRaw.(string)

	if !(len(roles) == 1 && roles[0].KybInitiated) {
		return c.JSON(403, map[string]interface{}{"message": "kyb completed or unauthorized", "data": nil})
	}

	kybdata, err := h.businessService.GetUserKYBInitiatedStatus(c.Request().Context(), userId)
	if err != nil {
		h.logger.Errorf("handler: %v", err)
		return c.JSON(500, map[string]interface{}{"message": "Failed to get KYB status", "data": nil})
	}

	return c.JSON(200, map[string]interface{}{"message": "Fetched kyb data", "data": kybdata})
}

func (h *BusinessHandler) UploadKYBDocuments(c echo.Context) error {
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)

	if !(len(roles) == 1 && roles[0].KybInitiated) {
		return c.JSON(403, map[string]interface{}{"message": "kyb completed or unauthorized", "data": nil})
	}

	fileuploader, err := fileupload.NewS3ClientFromConfig(*h.S3Config)
	if err != nil {
		h.logger.Errorf("Error creating S3 client: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to create S3 client"})
	}

	key := fmt.Sprintf("PII/%s/%s", roles[0].BID, "business_deed")
	// right now i think file name and content type are redundant as we are using presigned urls
	// will remove them later
	url, _, err := fileuploader.GetPresignedUploadURL(c.Request().Context(), "test.pdf", "application/pdf", key, 15*time.Minute)
	if err != nil {
		h.logger.Errorf("Error getting presigned upload URL: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to get presigned upload URL"})
	}

	return c.JSON(200, map[string]interface{}{"message": "Fetched presigned upload URL successfully", "data": map[string]interface{}{"url": url}})
}

func (h *BusinessHandler) ConfirmKYBDocumentsUpload(c echo.Context) error {
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)

	if !(len(roles) == 1 && roles[0].KybInitiated) {
		return c.JSON(403, map[string]interface{}{"message": "kyb completed or unauthorized", "data": nil})
	}

	fileuploader, err := fileupload.NewS3ClientFromConfig(*h.S3Config)
	if err != nil {
		h.logger.Errorf("Error creating S3 client: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to create S3 client"})
	}

	key := fmt.Sprintf("PII/%s/%s", roles[0].BID, "business_deed.pdf")
	fileurl, err := fileuploader.ConfirmUpload(c.Request().Context(), key, 15*time.Minute)
	if err != nil {
		h.logger.Errorf("Error confirming upload: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to confirm upload"})
	}

	err = h.businessService.UpdateBusinessDeedDocumentKey(c.Request().Context(), roles[0].BID, key)
	if err != nil {
		h.logger.Errorf("Error updating business deed document key: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to update business deed document key"})
	}

	return c.JSON(200, map[string]interface{}{"message": "Confirmed KYB documents upload successfully", "data": map[string]interface{}{"url": fileurl}})
}
