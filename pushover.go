package main

import (
	"fmt"
	"log"

	"github.com/gregdel/pushover"
)

// SendPushoverNotification sends a notification via Pushover.
// It returns the receipt ID if the message was an emergency priority and successfully sent, otherwise an empty string.
func SendPushoverNotification(config *Config, ruleAction *RuleActions, messageContent string, discordMessageLink string) (string, error) {
	if config.PushoverAppKey == "" {
		return "", fmt.Errorf("pushover AppKey is missing from global config")
	}
	if ruleAction.PushoverDestination == "" {
		return "", fmt.Errorf("pushoverDestination is missing from rule action")
	}

	log.Printf("Preparing Pushover notification for destination '%s' with app key '%s'", ruleAction.PushoverDestination, config.PushoverAppKey)

	// Create a new Pushover app instance
	app := pushover.New(config.PushoverAppKey)

	// Create a new recipient
	recipient := pushover.NewRecipient(ruleAction.PushoverDestination)

	// Create the message
	title := "Discord Notification" // Or make this configurable later
	fullMessage := fmt.Sprintf("%s\n\nDiscord Link: %s", messageContent, discordMessageLink)
	log.Printf("Pushover message content (first 50 chars): %.50s", fullMessage) // Log snippet of message
	message := pushover.NewMessageWithTitle(fullMessage, title)

	// Set priority
	// Pushover library uses these constants:
	// PriorityLowest, PriorityLow, PriorityNormal, PriorityHigh, PriorityEmergency
	switch ruleAction.Priority {
	case -2:
		message.Priority = pushover.PriorityLowest
	case -1:
		message.Priority = pushover.PriorityLow
	case 0: // Default to normal if 0 or not specified
		message.Priority = pushover.PriorityNormal
	case 1:
		message.Priority = pushover.PriorityHigh
	case 2:
		message.Priority = pushover.PriorityEmergency
		if ruleAction.Emergency != nil {
			message.Retry = ruleAction.Emergency.Retry
			message.Expire = ruleAction.Emergency.Expire
			// The gregdel/pushover library doesn't seem to have an explicit field for emergency sound.
			// Typically, the sound is tied to the client or priority.
			// Some libraries might allow specifying a sound, but this one defaults to Pushover's behavior for emergency.
		} else {
			// This case should ideally be prevented by config validation,
			// but as a fallback, send as high priority if emergency params are missing.
			log.Printf("Warning: Rule action has emergency priority (2) but Emergency parameters are missing. Sending as High Priority for rule action affecting destination %s.", ruleAction.PushoverDestination)
			message.Priority = pushover.PriorityHigh
		}
	default:
		log.Printf("Warning: Unknown priority %d specified for destination %s, defaulting to Normal Priority.", ruleAction.Priority, ruleAction.PushoverDestination)
		message.Priority = pushover.PriorityNormal
	}
	log.Printf("Set Pushover priority to %d for destination %s.", message.Priority, ruleAction.PushoverDestination)

	// Send the message
	log.Printf("Sending Pushover notification to %s...", ruleAction.PushoverDestination)
	resp, err := app.SendMessage(message, recipient)
	if err != nil {
		log.Printf("Error sending Pushover notification to %s: %v", ruleAction.PushoverDestination, err)
		return "", fmt.Errorf("failed to send Pushover notification: %w", err)
	}

	if resp.Status != 1 {
		log.Printf("Pushover API returned non-success status (%d) for destination %s. Errors: %v", resp.Status, ruleAction.PushoverDestination, resp.Errors)
		return "", fmt.Errorf("pushover API error for destination %s: status %d, errors: %v", ruleAction.PushoverDestination, resp.Status, resp.Errors)
	}

	log.Printf("Pushover notification sent successfully to %s. Message ID: %s", ruleAction.PushoverDestination, resp.ID)

	if message.Priority == pushover.PriorityEmergency {
		log.Printf("Emergency notification sent, Pushover receipt ID: %s for destination %s", resp.Receipt, ruleAction.PushoverDestination)
		return resp.Receipt, nil
	}

	return "", nil
}

// CheckPushoverReceipt checks the status of a Pushover emergency notification receipt.
func CheckPushoverReceipt(appKey string, receiptID string) (isAcknowledged bool, err error) {
	if appKey == "" {
		return false, fmt.Errorf("appKey is missing for checking Pushover receipt %s", receiptID)
	}
	if receiptID == "" {
		return false, fmt.Errorf("receiptID is missing for checking Pushover receipt with appKey %s", appKey)
	}
	// log.Printf("Checking Pushover receipt status for ID: %s with appKey: %s", receiptID, appKey) // Too verbose for every 5s poll

	// The gregdel/pushover library's App struct holds the token.
	// However, GetReceipt is a function in the pushover package, not a method on App.
	// It requires the app token (which is our appKey) and receiptID.
	// pushover.GetReceipt(token, receipt string) (*ReceiptDetails, error)
	
	// Note: The library's `pushover.New(appKey)` creates an `App` instance,
	// but `GetReceipt` is a package-level function that takes the token directly.
	// So, we don't need to instantiate an `App` here if we only use `GetReceipt`.
	// However, the `token` parameter for `GetReceipt` is indeed the Application's API token.

	details, err := pushover.GetReceipt(appKey, receiptID)
	if err != nil {
		// Check for specific Pushover API errors if necessary, e.g., receipt not found might be a specific error code.
		// For now, just return the error.
		return false, fmt.Errorf("failed to get Pushover receipt details for %s: %w", receiptID, err)
	}

	// According to Pushover API docs:
	// acknowledged: 1 if acknowledged, 0 otherwise
	// acknowledged_by: user key of the user that acknowledged
	// acknowledged_at: UNIX timestamp of acknowledgement time
	// last_delivered_at: UNIX timestamp of when the notification was last sent (for retrying notifications)
	// expired: 1 if notification has expired, 0 otherwise
	// expires_at: UNIX timestamp of when the notification will expire
	// called_back: 1 if a callback URL was called, 0 otherwise
	// called_back_at: UNIX timestamp of callback time

	// The library's ReceiptDetails struct has:
	// type ReceiptDetails struct {
	// 	 Status          int    `json:"status"`
	// 	 Acknowledged    int    `json:"acknowledged"` // This is what we need
	// 	 AcknowledgedBy  string `json:"acknowledged_by"`
	// 	 AcknowledgedAt  int    `json:"acknowledged_at"`
	// 	 LastDeliveredAt int    `json:"last_delivered_at"`
	// 	 Expired         int    `json:"expired"`
	// 	 ExpiresAt       int    `json:"expires_at"`
	// 	 CalledBack      int    `json:"called_back"`
	// 	 CalledBackAt    int    `json:"called_back_at"`
	// 	 Request         string `json:"request"`
	// 	 Errors          Errors `json:"errors"`
	// }
	// We need to check `details.Acknowledged == 1`.

	return details.Acknowledged == 1, nil
}
