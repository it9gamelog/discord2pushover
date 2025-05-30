package main

import (
	"fmt"
	"math" // Added for MaxInt32
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ProcessRules iterates through the configured rules and processes the first one that matches.
// previouslyNotifiedRulePriority helps avoid duplicate Pushover notifications if a bot reaction triggered the update.
func ProcessRules(message *discordgo.MessageCreate, config *Config, session DiscordSessionInterface, previouslyNotifiedRulePriority int) {
	log.Infof("Processing rules for message ID %s (user: %s, channel: %s). Previously notified priority: %d", message.ID, message.Author.Username, message.ChannelID, previouslyNotifiedRulePriority)
	for i, rule := range config.Rules {
		ruleNameLog := rule.Name
		if ruleNameLog == "" {
			ruleNameLog = fmt.Sprintf("unnamed_rule_%d", i+1)
		}
		log.Debugf("Evaluating rule #%d: '%s' for message ID %s", i+1, ruleNameLog, message.ID)

		conditionsMet := checkRuleConditions(message, &rule.Conditions, session, ruleNameLog)
		if conditionsMet {
			log.Infof("Rule #%d ('%s') MATCHED for message ID %s.", i+1, ruleNameLog, message.ID)
			// Construct Discord message link
			var discordMessageURL string
			if message.GuildID != "" {
				discordMessageURL = fmt.Sprintf("https://discord.com/channels/%s/%s/%s", message.GuildID, message.ChannelID, message.ID)
			} else {
				discordMessageURL = fmt.Sprintf("https://discord.com/channels/@me/%s/%s", message.ChannelID, message.ID)
			}

			// Trigger actions
			log.Infof("Triggering actions for matched rule '%s' on message ID %s", ruleNameLog, message.ID)

			// Suppress duplicate Pushover notifications
			// Pushover priorities: -2 (lowest) to 2 (emergency). Lower number = higher priority.
			// If current rule's priority is same or lower (numerically greater or equal) than a previously notified one, skip Pushover.
			sendNotification := true
			if rule.Actions.PushoverDestination != "" { // Only consider suppression if a destination is set
				if previouslyNotifiedRulePriority != math.MaxInt32 && rule.Actions.Priority >= previouslyNotifiedRulePriority {
					log.Warnf("Suppressing Pushover notification for rule '%s' (Priority: %d) on message ID %s. A notification with higher or equal priority (%d) was likely already sent due to bot reaction.",
						ruleNameLog, rule.Actions.Priority, message.ID, previouslyNotifiedRulePriority)
					sendNotification = false
				}
			} else {
				log.Debugf("Rule '%s' has no Pushover destination defined. No Pushover notification to send or suppress.", ruleNameLog)
				sendNotification = false // No destination means no notification to send
			}

			var receiptID string
			var errPushover error

			if sendNotification {
				receiptID, errPushover = SendPushoverNotification(config, &rule.Actions, message.Content, discordMessageURL)
				if errPushover != nil {
					log.Errorf("Error sending Pushover notification for rule '%s' (message ID %s): %v", ruleNameLog, message.ID, errPushover)
				} else {
					log.Infof("Pushover notification sent for rule '%s' (message ID %s). Receipt ID (if emergency): '%s'", ruleNameLog, message.ID, receiptID)
				}
			}

			// Handle standard reaction emoji for the rule, regardless of Pushover send status,
			// unless this reaction emoji itself was the one that triggered this evaluation pass
			// and we want to avoid re-adding it. For now, always attempt reaction if specified.
			// The `MessageReactionAdd` function in discordgo is idempotent (won't add if already present by bot).
			if rule.Actions.ReactionEmoji != "" {
				log.Debugf("Attempting to add reaction emoji '%s' for rule '%s' to message %s", rule.Actions.ReactionEmoji, ruleNameLog, message.ID)
				// Pass empty opts for now
				errReact := session.MessageReactionAdd(message.ChannelID, message.ID, rule.Actions.ReactionEmoji)
				if errReact != nil {
					log.Errorf("Error adding reaction emoji '%s' for rule '%s' (message %s): %v",
						rule.Actions.ReactionEmoji, ruleNameLog, message.ID, errReact)
				} else {
					log.Debugf("Successfully added reaction emoji '%s' for rule '%s' to message %s.",
						rule.Actions.ReactionEmoji, ruleNameLog, message.ID)
				}
			}

			// Handle emergency notification tracking if a receipt ID was returned (meaning notification was sent)
			if sendNotification && errPushover == nil && receiptID != "" { // Check sendNotification and no error
				if receiptID != "" && rule.Actions.Priority == 2 && rule.Actions.Emergency != nil {
					expiryDuration := time.Duration(rule.Actions.Emergency.Expire) * time.Second
					if rule.Actions.Emergency.Expire <= 0 { // Ensure non-negative, non-zero expiry for tracking
						log.Warnf("Rule '%s' has emergency priority but invalid 'expire' value (%d). Using default 1 hour for internal tracking.", ruleNameLog, rule.Actions.Emergency.Expire)
						expiryDuration = 3600 * time.Second
					}

					trackedMsg := TrackedEmergencyMessage{
						DiscordMessageID:  message.ID,
						DiscordChannelID:  message.ChannelID,
						PushoverReceiptID: receiptID,
						AckEmoji:          rule.Actions.Emergency.AckEmoji,
						ExpiryTime:        time.Now().Add(expiryDuration),
					}
					trackedMessages.Store(receiptID, trackedMsg)
					log.Infof("Tracking emergency message for rule '%s' (Receipt: %s, DiscordMsg: %s, AckEmoji: %s, Expires: %s)",
						ruleNameLog, receiptID, message.ID, trackedMsg.AckEmoji, trackedMsg.ExpiryTime.Format(time.RFC3339))
				} else if sendNotification && errPushover == nil && receiptID != "" && rule.Actions.Priority == 2 && rule.Actions.Emergency == nil {
					log.Warnf("Rule '%s' is emergency priority but 'emergency' parameters are not defined. Cannot track acknowledgement, despite notification being sent.", ruleNameLog)
				}
			}
			// Stop processing further rules for this message
			log.Infof("Finished processing actions for matched rule '%s' on message ID %s. No further rules will be evaluated for this message.", ruleNameLog, message.ID)
			return
		}
		log.Debugf("Rule #%d ('%s') did not match for message ID %s.", i+1, ruleNameLog, message.ID)
	}
	log.Infof("No rules matched for message ID %s after evaluating all %d rules.", message.ID, len(config.Rules))
}

// checkRuleConditions evaluates all conditions for a single rule using AND logic.
// A condition is considered "active" if its corresponding field in the config is non-zero.
// If a condition is active, it must evaluate to true. If not active, it's skipped (effectively true).
func checkRuleConditions(message *discordgo.MessageCreate, conditions *RuleConditions, session DiscordSessionInterface, ruleNameLog string) bool {
	logPrefix := fmt.Sprintf("Rule '%s', MessageID '%s': ", ruleNameLog, message.ID) // Keep this prefix for readability in logs

	// ChannelID condition
	if conditions.ChannelID != "" {
		if message.ChannelID != conditions.ChannelID {
			log.Debugf(logPrefix+"Condition failed (ChannelID): message channel %s != rule channel %s", message.ChannelID, conditions.ChannelID)
			return false
		}
		log.Debugf(logPrefix+"Condition passed (ChannelID): %s", conditions.ChannelID)
	}

	// MessageHasEmoji condition (checks reactions on the message)
	if len(conditions.MessageHasEmoji) > 0 {
		allRequiredEmojisFound := true // Assume true, prove false
		for _, reactionEmojiName := range conditions.MessageHasEmoji {
			singleRequiredEmojiFound := false
			for _, reaction := range message.Reactions { // message is *discordgo.MessageCreate, so message.Reactions is fullMessage.Reactions
				if reaction.Emoji.Name == reactionEmojiName {
					// If ReactToAtMention is a condition for this rule, and the bot ("Me") added this specific reaction,
					// then this specific reaction instance doesn't count towards fulfilling MessageHasEmoji.
					if conditions.ReactToAtMention && reaction.Me {
						log.Debugf(logPrefix+"Reaction emoji '%s' found, but ignored for MessageHasEmoji because ReactToAtMention is true and bot added this reaction (reaction.Me is true).", reaction.Emoji.Name)
						continue // This specific reaction instance (by the bot) doesn't count. Check other instances of the same emoji.
					}
					singleRequiredEmojiFound = true
					log.Debugf(logPrefix+"Condition MessageHasEmoji: Found required reaction emoji '%s' (reaction.Me: %t).", reaction.Emoji.Name, reaction.Me)
					break // Found this specific required emoji
				}
			}
			if !singleRequiredEmojiFound {
				allRequiredEmojisFound = false // This required emoji was not found (or was ignored due to bot+mention interaction)
				log.Debugf(logPrefix+"Condition failed (MessageHasEmoji): Required reaction emoji '%s' not found or was ignored.", reactionEmojiName)
				break // No need to check other required emojis
			}
		}
		if !allRequiredEmojisFound {
			// Log which emojis were required, and what reactions are present for debugging
			presentEmojis := []string{}
			for _, r := range message.Reactions {
				presentEmojis = append(presentEmojis, fmt.Sprintf("%s (Me:%t)", r.Emoji.Name, r.Me))
			}
			log.Debugf(logPrefix+"Condition failed (MessageHasEmoji): Not all required emojis %v found or applicable. Present reactions: [%s]", conditions.MessageHasEmoji, strings.Join(presentEmojis, ", "))
			return false
		}
		log.Debugf(logPrefix+"Condition passed (MessageHasEmoji): All required emojis %v found and applicable.", conditions.MessageHasEmoji)
	}

	// ContentIncludes condition (ALL keywords must be present)
	if len(conditions.ContentIncludes) > 0 {
		allKeywordsFound := true
		lowerMessageContent := strings.ToLower(message.Content) // Optimize: convert message content to lower once
		for _, keyword := range conditions.ContentIncludes {
			if !strings.Contains(lowerMessageContent, strings.ToLower(keyword)) {
				allKeywordsFound = false
				log.Debugf(logPrefix+"Condition failed (ContentIncludes): keyword '%s' not found in message.", keyword)
				break
			}
		}
		if !allKeywordsFound {
			return false
		}
		log.Debugf(logPrefix+"Condition passed (ContentIncludes): All keywords %v found.", conditions.ContentIncludes)
	}

	// Mentions conditions: ReactToAtMention and SpecificMentions
	// These are treated as separate AND conditions if configured.

	// ReactToAtMention condition
	if conditions.ReactToAtMention {
		botMentioned := false
		currentSessionState := session.State() // Call State() once
		if currentSessionState == nil || currentSessionState.User == nil {
			log.Warnf(logPrefix + "ReactToAtMention check: Bot user ID not available from session state. Condition will fail.")
			// Fail the condition if bot ID cannot be determined
			botMentioned = false
		} else {
			botID := currentSessionState.User.ID
			for _, user := range message.Mentions {
				if user.ID == botID {
					botMentioned = true
					break
				}
			}
		}

		if !botMentioned {
			botIDForLog := "unavailable"
			if currentSessionState != nil && currentSessionState.User != nil {
				botIDForLog = currentSessionState.User.ID
			}
			log.Debugf(logPrefix+"Condition failed (ReactToAtMention): Bot (ID: %s) was not mentioned in message content.", botIDForLog)
			return false
		}
		log.Debugf(logPrefix + "Condition passed (ReactToAtMention): Bot was mentioned in message content.")
	}

	// SpecificMentions condition
	if len(conditions.SpecificMentions) > 0 {
		specificMentionFound := false
		for _, mentionID := range conditions.SpecificMentions {
			// Check user mentions
			for _, user := range message.Mentions {
				if user.ID == mentionID {
					specificMentionFound = true
					log.Debugf(logPrefix+"SpecificMentions: Found mentioned user ID %s.", mentionID)
					break
				}
			}
			if specificMentionFound {
				break
			}
			// Check role mentions
			for _, roleID := range message.MentionRoles {
				if roleID == mentionID {
					specificMentionFound = true
					log.Debugf(logPrefix+"SpecificMentions: Found mentioned role ID %s.", mentionID)
					break
				}
			}
			if specificMentionFound {
				break
			}
		}
		if !specificMentionFound {
			log.Debugf(logPrefix+"Condition failed (SpecificMentions): None of the specified users/roles %v were mentioned.", conditions.SpecificMentions)
			return false
		}
		log.Debugf(logPrefix+"Condition passed (SpecificMentions): At least one of %v was mentioned.", conditions.SpecificMentions)
	}

	// If all active conditions passed (or no conditions were active), the rule conditions are met.
	log.Debugf(logPrefix + "All active conditions passed for rule.")
	return true
}
