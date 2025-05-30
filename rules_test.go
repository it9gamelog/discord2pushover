package main

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

// mockSessionForRulesTest creates a mock session for testing rules.
func mockSessionForRulesTest(botID string) DiscordSessionInterface {
	if botID == "" {
		botID = "bot_rules_test_id"
	}
	st := &discordgo.State{}
	st.User = &discordgo.User{ID: botID}

	mock := &MockDiscordSession{
		TestStateOverride: st,
	}
	return mock
}

func TestCheckRuleConditions_MessageHasEmoji_Logic(t *testing.T) {
	if log == nil {
		log = logrus.New()
	}
	originalLogOut := log.Out
	originalLogLevel := log.GetLevel()
	var testBuf bytes.Buffer
	defer func() {
		log.SetOutput(originalLogOut)
		log.SetLevel(originalLogLevel)
	}()
	log.SetOutput(&testBuf)
	log.SetLevel(logrus.DebugLevel)

	session := mockSessionForRulesTest("testBotID") // Consistent bot ID

	// Base message, reactions will be overridden in test cases
	// Now returns *discordgo.Message directly
	baseMessageFunc := func(reactions []*discordgo.MessageReactions) *discordgo.Message {
		return &discordgo.Message{
			ID:        "testMsgEmoji",
			ChannelID: "testChannelEmoji",
			Author:    &discordgo.User{ID: "user"},
			Mentions:  []*discordgo.User{{ID: "testBotID"}}, // Bot is mentioned for ReactToAtMention cases
			Reactions: reactions,
		}
	}

	tests := []struct {
		name             string
		conditions       RuleConditions
		messageReactions []*discordgo.MessageReactions
		expectedResult   bool
		expectedLog      []string // Substrings to check for in log
	}{
		// --- ANY OF LOGIC ---
		{
			name: "AnyOf: OneMatch (A of [A,B])",
			conditions: RuleConditions{MessageHasEmoji: []string{"🅰️", "🅱️"}},
			messageReactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🅰️"}, Me: false},
			},
			expectedResult: true,
			expectedLog:    []string{"Condition MessageHasEmoji: Found matching reaction emoji '🅰️'", "Condition met (ANY of)"},
		},
		{
			name: "AnyOf: OneMatch (B of [A,B])",
			conditions: RuleConditions{MessageHasEmoji: []string{"🅰️", "🅱️"}},
			messageReactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🅱️"}, Me: false},
			},
			expectedResult: true,
			expectedLog:    []string{"Condition MessageHasEmoji: Found matching reaction emoji '🅱️'", "Condition met (ANY of)"},
		},
		{
			name: "AnyOf: MultipleMatches (A,B of [A,B])",
			conditions: RuleConditions{MessageHasEmoji: []string{"🅰️", "🅱️"}},
			messageReactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🅰️"}, Me: false},
				{Emoji: &discordgo.Emoji{Name: "🅱️"}, Me: false},
			},
			expectedResult: true, // Stops at first match (🅰️)
			expectedLog:    []string{"Condition MessageHasEmoji: Found matching reaction emoji '🅰️'", "Condition met (ANY of)"},
		},
		{
			name: "AnyOf: NoMatch (C on msg, [A,B] in rule)",
			conditions: RuleConditions{MessageHasEmoji: []string{"🅰️", "🅱️"}},
			messageReactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🇨"}, Me: false},
			},
			expectedResult: false,
			expectedLog:    []string{"Condition failed (MessageHasEmoji): None of the required emojis [🅰️ 🅱️] were found"},
		},
		{
			name: "AnyOf: EmptyReactionsOnMsg",
			conditions: RuleConditions{MessageHasEmoji: []string{"🅰️", "🅱️"}},
			messageReactions: []*discordgo.MessageReactions{},
			expectedResult:   false,
			expectedLog:      []string{"Condition failed (MessageHasEmoji): None of the required emojis [🅰️ 🅱️] were found"},
		},
		// --- Interaction with ReactToAtMention ---
		{
			name: "AnyOf_ReactToMention: BotReactedMatch (A of [A,B]), Ignored",
			conditions: RuleConditions{ReactToAtMention: true, MessageHasEmoji: []string{"🅰️", "🅱️"}},
			messageReactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🅰️"}, Me: true}, // Bot reaction
			},
			expectedResult: false, // Bot's "🅰️" is ignored
			expectedLog:    []string{"MessageHasEmoji: Candidate reaction emoji '🅰️' found (added by bot, reaction.Me=true), but will be ignored", "Condition failed (MessageHasEmoji): None of the required emojis [🅰️ 🅱️] were found"},
		},
		{
			name: "AnyOf_ReactToMention: BotReacted_A_Ignored, UserReacted_B_Match (A,B of [A,B])",
			conditions: RuleConditions{ReactToAtMention: true, MessageHasEmoji: []string{"🅰️", "🅱️"}},
			messageReactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🅰️"}, Me: true},  // Bot reaction, ignored
				{Emoji: &discordgo.Emoji{Name: "🅱️"}, Me: false}, // User reaction, matches
			},
			expectedResult: true,
			expectedLog:    []string{"MessageHasEmoji: Candidate reaction emoji '🅰️' found (added by bot, reaction.Me=true), but will be ignored", "Condition MessageHasEmoji: Found matching reaction emoji '🅱️'", "Condition met (ANY of)"},
		},
		{
			name: "AnyOf_NoReactToMention: BotReactedMatch (A of [A,B]), NotIgnored",
			conditions: RuleConditions{ReactToAtMention: false, MessageHasEmoji: []string{"🅰️", "🅱️"}}, // ReactToAtMention is false
			messageReactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🅰️"}, Me: true}, // Bot reaction, but not ignored
			},
			expectedResult: true,
			expectedLog:    []string{"Condition MessageHasEmoji: Found matching reaction emoji '🅰️'", "Condition met (ANY of)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testBuf.Reset()
			msg := baseMessageFunc(tt.messageReactions) // Use the new func name

			// Ensure other conditions don't interfere if not specified
			if tt.conditions.ChannelID == "" { // Default to pass if not set
				tt.conditions.ChannelID = msg.ChannelID
			}


			result := checkRuleConditions(msg, &tt.conditions, session, tt.name)
			if result != tt.expectedResult {
				t.Errorf("Test '%s': Expected result %v, got %v", tt.name, tt.expectedResult, result)
			}

			logOutput := testBuf.String()
			for _, logSubstr := range tt.expectedLog {
				if !strings.Contains(logOutput, logSubstr) {
					t.Errorf("Test '%s': Expected log substring '%s' not found. Full log:\n%s", tt.name, logSubstr, logOutput)
				}
			}
		})
	}
}


func TestProcessRules_NotificationSuppression(t *testing.T) {
	if log == nil {
		log = logrus.New()
	}
	originalLogOut := log.Out
	originalLogLevel := log.GetLevel()
	var testLogCap bytes.Buffer // Used to capture all log output for assertions

	// Setup test hook for SendPushoverNotification
	originalTestHookDisablePushoverSend := testHookDisablePushoverSend
	testHookDisablePushoverSend = true // Disable actual Pushover sends for these tests

	defer func() {
		log.SetOutput(originalLogOut)
		log.SetLevel(originalLogLevel)
		testHookDisablePushoverSend = originalTestHookDisablePushoverSend // Restore hook
		testHookPushoverSendCalled = false // Reset for other tests if any
	}()
	log.SetOutput(&testLogCap)
	log.SetLevel(logrus.DebugLevel)

	mockSession := mockSessionForRulesTest("bot_process_rules_id")

	// baseMsg is now *discordgo.Message
	baseMsg := &discordgo.Message{
		ID:        "msgProcRules",
		ChannelID: "chProcRules",
		GuildID:   "guildProcRules",
		Author:    &discordgo.User{ID: "userProcRules"},
		Content:   "Test message for ProcessRules",
		// Reactions field can be added if specific tests need a starting reaction state
	}

	tests := []struct {
		name                         string
		rule                         Rule
		previouslyNotifiedRulePriority int
		configPushoverAppKey         string
		expectSuppressionLog         bool
		expectPushoverSendLog        bool
		expectReactionAddLog         bool
	}{
		{
			name: "Notify_PrioMaxInt32",
			rule: Rule{Name: "TestRule1", Conditions: RuleConditions{ChannelID: "chProcRules"}, Actions: RuleActions{Priority: 0, PushoverDestination: "userkey", ReactionEmoji: "👍"}},
			previouslyNotifiedRulePriority: math.MaxInt32,
			configPushoverAppKey:         "fakeAppKey",
			expectSuppressionLog:         false,
			expectPushoverSendLog:        true,
			expectReactionAddLog:         true,
		},
		{
			name: "Notify_CurrentPrioHigher",
			rule: Rule{Name: "TestRule2", Conditions: RuleConditions{ChannelID: "chProcRules"}, Actions: RuleActions{Priority: -1, PushoverDestination: "userkey", ReactionEmoji: "👍"}},
			previouslyNotifiedRulePriority: 0,
			configPushoverAppKey:         "fakeAppKey",
			expectSuppressionLog:         false,
			expectPushoverSendLog:        true,
			expectReactionAddLog:         true,
		},
		{
			name: "Suppress_CurrentPrioEqual",
			rule: Rule{Name: "TestRule3", Conditions: RuleConditions{ChannelID: "chProcRules"}, Actions: RuleActions{Priority: 0, PushoverDestination: "userkey", ReactionEmoji: "👍"}},
			previouslyNotifiedRulePriority: 0,
			configPushoverAppKey:         "fakeAppKey",
			expectSuppressionLog:         true,
			expectPushoverSendLog:        false,
			expectReactionAddLog:         true,
		},
		{
			name: "Suppress_CurrentPrioLower",
			rule: Rule{Name: "TestRule4", Conditions: RuleConditions{ChannelID: "chProcRules"}, Actions: RuleActions{Priority: 1, PushoverDestination: "userkey", ReactionEmoji: "👍"}},
			previouslyNotifiedRulePriority: 0,
			configPushoverAppKey:         "fakeAppKey",
			expectSuppressionLog:         true,
			expectPushoverSendLog:        false,
			expectReactionAddLog:         true,
		},
		{
			name: "NoPushover_NoDestination",
			rule: Rule{Name: "TestRule5", Conditions: RuleConditions{ChannelID: "chProcRules"}, Actions: RuleActions{Priority: 0, PushoverDestination: "", ReactionEmoji: "👍"}},
			previouslyNotifiedRulePriority: math.MaxInt32,
			configPushoverAppKey:         "fakeAppKey",
			expectSuppressionLog:         false,
			expectPushoverSendLog:        false,
			expectReactionAddLog:         true,
		},
		{
			name: "NoPushover_NoAppKey",
			rule: Rule{Name: "TestRule6", Conditions: RuleConditions{ChannelID: "chProcRules"}, Actions: RuleActions{Priority: 0, PushoverDestination: "userkey", ReactionEmoji: "👍"}},
			previouslyNotifiedRulePriority: math.MaxInt32,
			configPushoverAppKey:         "",
			expectSuppressionLog:         false,
			expectPushoverSendLog:        false,
			expectReactionAddLog:         true,
		},
		{
			name: "NoReactionEmoji",
			rule: Rule{Name: "TestRule7", Conditions: RuleConditions{ChannelID: "chProcRules"}, Actions: RuleActions{Priority: 0, PushoverDestination: "userkey"}},
			previouslyNotifiedRulePriority: math.MaxInt32,
			configPushoverAppKey:         "fakeAppKey",
			expectSuppressionLog:         false,
			expectPushoverSendLog:        true,
			expectReactionAddLog:         false,
		},
	}

	originalGlobalCfg := globalConfig
	defer func() { globalConfig = originalGlobalCfg }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogCap.Reset()
			testHookPushoverSendCalled = false

			globalConfig = &Config{
				PushoverAppKey: tt.configPushoverAppKey,
				Rules:          []Rule{tt.rule},
			}

			ProcessRules(baseMsg, globalConfig, mockSession, tt.previouslyNotifiedRulePriority)
			logOutput := testLogCap.String()

			suppressionLogExpected := fmt.Sprintf("Suppressing Pushover notification for rule '%s'", tt.rule.Name)
			if tt.expectSuppressionLog {
				if !strings.Contains(logOutput, suppressionLogExpected) {
					t.Errorf("Expected suppression log ('%s') not found. Log: %s", suppressionLogExpected, logOutput)
				}
				if testHookPushoverSendCalled {
					t.Errorf("SendPushoverNotification was called (testHookPushoverSendCalled=true) but should have been suppressed. Log: %s", logOutput)
				}
			} else {
				if strings.Contains(logOutput, suppressionLogExpected) {
					t.Errorf("Unexpected suppression log ('%s') found. Log: %s", suppressionLogExpected, logOutput)
				}
				if tt.rule.Actions.PushoverDestination != "" && tt.configPushoverAppKey != "" {
					if !testHookPushoverSendCalled && tt.expectPushoverSendLog {
						t.Errorf("SendPushoverNotification was NOT called (testHookPushoverSendCalled=false) but was expected to be. Log: %s", logOutput)
					}
					pushoverActuallySentLog := fmt.Sprintf("Pushover notification sent for rule '%s'", tt.rule.Name)
					if tt.expectPushoverSendLog && !strings.Contains(logOutput, pushoverActuallySentLog) {
						t.Errorf("Expected Pushover 'sent' log ('%s') not found. Log: %s", pushoverActuallySentLog, logOutput)
					}
					if !tt.expectPushoverSendLog && strings.Contains(logOutput, pushoverActuallySentLog) {
                         t.Errorf("Unexpected Pushover 'sent' log ('%s') found. Log: %s", pushoverActuallySentLog, logOutput)
                    }

				} else if tt.expectPushoverSendLog {
					t.Errorf("Test logic error: expectPushoverSendLog is true but no destination/appkey, so send couldn't happen. Rule: %s", tt.rule.Name)
				}
			}

			reactionAddLogExpected := fmt.Sprintf("MockDiscordSession: MessageReactionAdd called with: chID=%s, msgID=%s, emoji=%s", baseMsg.ChannelID, baseMsg.ID, tt.rule.Actions.ReactionEmoji)
			if tt.expectReactionAddLog {
				if !strings.Contains(logOutput, reactionAddLogExpected) {
					t.Errorf("Expected MessageReactionAdd log ('%s') not found. Log: %s", reactionAddLogExpected, logOutput)
				}
			} else {
				if tt.rule.Actions.ReactionEmoji != "" && strings.Contains(logOutput, reactionAddLogExpected) {
					t.Errorf("Unexpected MessageReactionAdd log ('%s') found. Log: %s", reactionAddLogExpected, logOutput)
				} else if tt.rule.Actions.ReactionEmoji == "" && strings.Contains(logOutput, "MockDiscordSession: MessageReactionAdd called") {
                    t.Errorf("Unexpected MessageReactionAdd log found when no ReactionEmoji was set. Log: %s", logOutput)
                }
			}
		})
	}
}
