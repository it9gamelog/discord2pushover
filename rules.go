package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ProcessRules iterates through the configured rules and processes the first one that matches.
func ProcessRules(message *discordgo.MessageCreate, config *Config, session *discordgo.Session) {
	log.Printf("Processing rules for message ID %s (user: %s, channel: %s)", message.ID, message.Author.Username, message.ChannelID)
	for i, rule := range config.Rules {
		ruleNameLog := rule.Name
		if ruleNameLog == "" {
			ruleNameLog = fmt.Sprintf("unnamed_rule_%d", i+1)
		}
		log.Printf("Evaluating rule #%d: '%s' for message ID %s", i+1, ruleNameLog, message.ID)

		conditionsMet := checkRuleConditions(message, &rule.Conditions, session, ruleNameLog)
		if conditionsMet {
			log.Printf("Rule #%d ('%s') MATCHED for message ID %s.", i+1, ruleNameLog, message.ID)
			// Construct Discord message link
			var discordMessageURL string
			if message.GuildID != "" {
				discordMessageURL = fmt.Sprintf("https://discord.com/channels/%s/%s/%s", message.GuildID, message.ChannelID, message.ID)
			} else {
				discordMessageURL = fmt.Sprintf("https://discord.com/channels/@me/%s/%s", message.ChannelID, message.ID)
			}

			// Trigger actions
			log.Printf("Triggering actions for matched rule '%s' on message ID %s", ruleNameLog, message.ID)
			receiptID, err := SendPushoverNotification(config, &rule.Actions, message.Content, discordMessageURL)
			if err != nil {
				log.Printf("Error sending Pushover notification for rule '%s' (message ID %s): %v", ruleNameLog, message.ID, err)
			} else {
				log.Printf("Pushover notification sent for rule '%s' (message ID %s). Receipt ID (if emergency): '%s'", ruleNameLog, message.ID, receiptID)

				// Handle standard reaction emoji for the rule, regardless of priority
				if rule.Actions.ReactionEmoji != "" {
					log.Printf("Attempting to add reaction emoji '%s' for rule '%s' to message %s", rule.Actions.ReactionEmoji, ruleNameLog, message.ID)
					errReact := session.MessageReactionAdd(message.ChannelID, message.ID, rule.Actions.ReactionEmoji)
					if errReact != nil {
						log.Printf("Error adding reaction emoji '%s' for rule '%s' (message %s): %v",
							rule.Actions.ReactionEmoji, ruleNameLog, message.ID, errReact)
					} else {
						log.Printf("Successfully added reaction emoji '%s' for rule '%s' to message %s.",
							rule.Actions.ReactionEmoji, ruleNameLog, message.ID)
					}
				}

				// Handle emergency notification tracking if a receipt ID was returned
				if receiptID != "" && rule.Actions.Priority == 2 && rule.Actions.Emergency != nil {
					expiryDuration := time.Duration(rule.Actions.Emergency.Expire) * time.Second
					if rule.Actions.Emergency.Expire <= 0 { // Ensure non-negative, non-zero expiry for tracking
						log.Printf("Warning: Rule '%s' has emergency priority but invalid 'expire' value (%d). Using default 1 hour for internal tracking.", ruleNameLog, rule.Actions.Emergency.Expire)
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
					log.Printf("Tracking emergency message for rule '%s' (Receipt: %s, DiscordMsg: %s, AckEmoji: %s, Expires: %s)",
						ruleNameLog, receiptID, message.ID, trackedMsg.AckEmoji, trackedMsg.ExpiryTime.Format(time.RFC3339))
				} else if receiptID != "" && rule.Actions.Priority == 2 && rule.Actions.Emergency == nil {
					log.Printf("Warning: Rule '%s' is emergency priority but 'emergency' parameters are not defined. Cannot track acknowledgement.", ruleNameLog)
				}
			}
			// Stop processing further rules for this message
			log.Printf("Finished processing actions for matched rule '%s' on message ID %s. No further rules will be evaluated for this message.", ruleNameLog, message.ID)
			return
		}
		log.Printf("Rule #%d ('%s') did not match for message ID %s.", i+1, ruleNameLog, message.ID)
	}
	log.Printf("No rules matched for message ID %s after evaluating all %d rules.", message.ID, len(config.Rules))
}

// checkRuleConditions evaluates all conditions for a single rule using AND logic.
// A condition is considered "active" if its corresponding field in the config is non-zero.
// If a condition is active, it must evaluate to true. If not active, it's skipped (effectively true).
func checkRuleConditions(message *discordgo.MessageCreate, conditions *RuleConditions, session *discordgo.Session, ruleNameLog string) bool {
	logPrefix := fmt.Sprintf("Rule '%s', MessageID '%s': ", ruleNameLog, message.ID)

	// ChannelID condition
	if conditions.ChannelID != "" {
		if message.ChannelID != conditions.ChannelID {
			log.Printf(logPrefix+"Condition failed (ChannelID): message channel %s != rule channel %s", message.ChannelID, conditions.ChannelID)
			return false
		}
		log.Printf(logPrefix+"Condition passed (ChannelID): %s", conditions.ChannelID)
	}

	// MessageHasEmoji condition (checks reactions on the message)
	if len(conditions.MessageHasEmoji) > 0 {
		foundReaction := false
		for _, reactionEmojiName := range conditions.MessageHasEmoji {
			for _, reaction := range message.Reactions {
				if reaction.Emoji.Name == reactionEmojiName {
					foundReaction = true
					break
				}
			}
			if foundReaction { // Found one of the required emojis
				break
			}
		}
		if !foundReaction {
			log.Printf(logPrefix+"Condition failed (MessageHasEmoji): No specified reaction emoji found. Required: %v, Present: %v", conditions.MessageHasEmoji, message.Reactions)
			return false
		}
		log.Printf(logPrefix+"Condition passed (MessageHasEmoji): Found one of %v", conditions.MessageHasEmoji)
	}

	// ContentIncludes condition (ALL keywords must be present)
	if len(conditions.ContentIncludes) > 0 {
		allKeywordsFound := true
		lowerMessageContent := strings.ToLower(message.Content) // Optimize: convert message content to lower once
		for _, keyword := range conditions.ContentIncludes {
			if !strings.Contains(lowerMessageContent, strings.ToLower(keyword)) {
				allKeywordsFound = false
				log.Printf(logPrefix+"Condition failed (ContentIncludes): keyword '%s' not found in message.", keyword)
				break
			}
		}
		if !allKeywordsFound {
			return false
		}
		log.Printf(logPrefix+"Condition passed (ContentIncludes): All keywords %v found.", conditions.ContentIncludes)
	}

	// Mentions conditions: ReactToAtMention and SpecificMentions
	// These are treated as separate AND conditions if configured.

	// ReactToAtMention condition
	if conditions.ReactToAtMention {
		botMentioned := false
		for _, user := range message.Mentions {
			if user.ID == session.State.User.ID {
				botMentioned = true
				break
			}
		}
		if !botMentioned {
			log.Printf(logPrefix+"Condition failed (ReactToAtMention): Bot (ID: %s) was not mentioned.", session.State.User.ID)
			return false
		}
		log.Printf(logPrefix + "Condition passed (ReactToAtMention): Bot was mentioned.")
	}

	// SpecificMentions condition
	if len(conditions.SpecificMentions) > 0 {
		specificMentionFound := false
		for _, mentionID := range conditions.SpecificMentions {
			// Check user mentions
			for _, user := range message.Mentions {
				if user.ID == mentionID {
					specificMentionFound = true
					log.Printf(logPrefix+"SpecificMentions: Found mentioned user ID %s.", mentionID)
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
					log.Printf(logPrefix+"SpecificMentions: Found mentioned role ID %s.", mentionID)
					break
				}
			}
			if specificMentionFound {
				break
			}
		}
		if !specificMentionFound {
			log.Printf(logPrefix+"Condition failed (SpecificMentions): None of the specified users/roles %v were mentioned.", conditions.SpecificMentions)
			return false
		}
		log.Printf(logPrefix+"Condition passed (SpecificMentions): At least one of %v was mentioned.", conditions.SpecificMentions)
	}

	// If all active conditions passed (or no conditions were active), the rule conditions are met.
	log.Printf(logPrefix + "All active conditions passed for rule.")
	return true
}
