package main

import (
	"fmt"
	"log"
	"time"

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
			message.Retry = time.Duration(ruleAction.Emergency.Retry) * time.Second
			message.Expire = time.Duration(ruleAction.Emergency.Expire) * time.Second
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
