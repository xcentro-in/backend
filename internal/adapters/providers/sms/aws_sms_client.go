package sms

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/pinpointsmsvoicev2"
	"github.com/aws/aws-sdk-go-v2/service/pinpointsmsvoicev2/types"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// AWSSMSClient implements SMSService interface for AWS End User Messaging SMS
type AWSSMSClient struct {
	client            *pinpointsmsvoicev2.Client
	defaultOriginationID string // Default sender ID
	region            string
}

// Ensure AWSSMSClient implements SMSService interface at compile time
var _ SMSService = (*AWSSMSClient)(nil)

// NewAWSSMSClientFromConfig builds AWS SMS client using structured application config
func NewAWSSMSClientFromConfig(c config.SMSConfig) (*AWSSMSClient, error) {
	if c.Region == "" {
		return nil, fmt.Errorf("sms region is required")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(c.Region)}

	if c.Profile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(c.Profile))
	}

	if c.Endpoint != "" {
		// Custom endpoint (e.g., localstack for testing)
		loadOpts = append(loadOpts, awsconfig.WithBaseEndpoint(c.Endpoint))
	}

	// Static credentials override if provided
	if c.AccessKey != "" && c.SecretKey != "" {
		staticCreds := credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, c.SessionToken)
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(staticCreds))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	return &AWSSMSClient{
		client:            pinpointsmsvoicev2.NewFromConfig(awsCfg),
		defaultOriginationID: c.OriginationID,
		region:            c.Region,
	}, nil
}

// SendSMS sends a single SMS message via AWS End User Messaging
func (s *AWSSMSClient) SendSMS(ctx context.Context, msg *SMSMessage) (messageID string, err error) {
	if err := msg.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	// Validate phone number format (basic E.164 check)
	if !strings.HasPrefix(msg.To, "+") {
		return "", ErrInvalidPhoneNumber
	}

	// Use default origination ID if not specified
	originationID := msg.OriginationID
	if originationID == "" {
		originationID = s.defaultOriginationID
	}

	// Build SendTextMessage input
	input := &pinpointsmsvoicev2.SendTextMessageInput{
		DestinationPhoneNumber: aws.String(msg.To),
		MessageBody:            aws.String(msg.Body),
		MessageType:            types.MessageTypeTransactional, // Transactional for OTP
	}

	// Add origination identity (sender ID) if provided
	if originationID != "" {
		input.OriginationIdentity = aws.String(originationID)
	}

	// Send SMS
	resp, err := s.client.SendTextMessage(ctx, input)
	if err != nil {
		logger.GetDefault().Errorf("Failed to send SMS to %s: %v", msg.To, err)
		return "", fmt.Errorf("failed to send SMS: %w", err)
	}

	messageID = aws.ToString(resp.MessageId)
	logger.GetDefault().Infof("SMS sent successfully to %s, Message ID: %s", msg.To, messageID)

	return messageID, nil
}
