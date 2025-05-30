package main

import (
	"fmt"
	"time"

	"github.com/gregdel/pushover"
)

// testHookDisablePushoverSend is for unit testing. If true, SendPushoverNotification returns success without actual sending.
var testHookDisablePushoverSend bool
// testHookPushoverSendCalled is for unit testing, to check if SendPushoverNotification's core logic was invoked.
var testHookPushoverSendCalled bool


// SendPushoverNotification sends a notification via Pushover.
// It returns the receipt ID if the message was an emergency priority and successfully sent, otherwise an empty string.
func SendPushoverNotification(config *Config, ruleAction *RuleActions, messageContent string, discordMessageLink string) (string, error) {
	testHookPushoverSendCalled = true // Mark that we entered the function for test verification
	if testHookDisablePushoverSend {
		log.Debug("testHookDisablePushoverSend is true, faking successful Pushover send.")
		// Simulate a successful emergency message for testing receipt ID path
		if ruleAction.Priority == 2 {
			return "fake-receipt-id-for-test", nil
		}
		return "", nil
	}

	if config.PushoverAppKey == "" {
		return "", fmt.Errorf("pushover AppKey is missing from global config")
	}
	if ruleAction.PushoverDestination == "" {
		return "", fmt.Errorf("pushoverDestination is missing from rule action")
	}

	log.Infof("Preparing Pushover notification for destination '%s' with app key '%s'", ruleAction.PushoverDestination, config.PushoverAppKey)

	// Create a new Pushover app instance
	app := pushover.New(config.PushoverAppKey)

	// Create a new recipient
	recipient := pushover.NewRecipient(ruleAction.PushoverDestination)

	// Create the message
	title := "Discord Notification" // Or make this configurable later
	fullMessage := fmt.Sprintf("%s\n\nDiscord Link: %s", messageContent, discordMessageLink)
	log.Debugf("Pushover message content (first 50 chars): %.50s", fullMessage) // Log snippet of message
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
			log.Warnf("Rule action has emergency priority (2) but Emergency parameters are missing. Sending as High Priority for rule action affecting destination %s.", ruleAction.PushoverDestination)
			message.Priority = pushover.PriorityHigh
		}
	default:
		log.Warnf("Unknown priority %d specified for destination %s, defaulting to Normal Priority.", ruleAction.Priority, ruleAction.PushoverDestination)
		message.Priority = pushover.PriorityNormal
	}
	log.Infof("Set Pushover priority to %d for destination %s.", message.Priority, ruleAction.PushoverDestination)

	// Send the message
	log.Infof("Sending Pushover notification to %s...", ruleAction.PushoverDestination)
	resp, err := app.SendMessage(message, recipient)
	if err != nil {
		log.Errorf("Error sending Pushover notification to %s: %v", ruleAction.PushoverDestination, err)
		return "", fmt.Errorf("failed to send Pushover notification: %w", err)
	}

	if resp.Status != 1 {
		log.Errorf("Pushover API returned non-success status (%d) for destination %s. Errors: %v", resp.Status, ruleAction.PushoverDestination, resp.Errors)
		return "", fmt.Errorf("pushover API error for destination %s: status %d, errors: %v", ruleAction.PushoverDestination, resp.Status, resp.Errors)
	}

	log.Infof("Pushover notification sent successfully to %s. Message ID: %s", ruleAction.PushoverDestination, resp.ID)

	if message.Priority == pushover.PriorityEmergency {
		log.Infof("Emergency notification sent, Pushover receipt ID: %s for destination %s", resp.Receipt, ruleAction.PushoverDestination)
		return resp.Receipt, nil
	}

	return "", nil
}
