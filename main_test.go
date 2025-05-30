package main

import (
	"bytes"
	// "flag" // No longer used directly in these tests for log level
	"fmt"
	"math" // For math.MaxInt32 in new tests
	"os"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

// TestLogLevelParsing (existing test)
func TestLogLevelParsing(t *testing.T) {
	tests := []struct {
		levelArg string
		expected logrus.Level
	}{
		{"trace", logrus.TraceLevel},
		{"debug", logrus.DebugLevel},
		{"info", logrus.InfoLevel},
		{"warn", logrus.WarnLevel},
		{"error", logrus.ErrorLevel},
		{"fatal", logrus.FatalLevel},
		{"panic", logrus.PanicLevel},
	}

	for _, tt := range tests {
		t.Run(tt.levelArg, func(t *testing.T) {
			level, err := logrus.ParseLevel(tt.levelArg)
			if err != nil {
				t.Fatalf("Failed to parse log level string '%s': %v", tt.levelArg, err)
			}

			if level != tt.expected {
				t.Errorf("For arg '%s': expected logrus level %v, but parsed to %v", tt.levelArg, tt.expected, level)
			}

			testLogger := logrus.New()
			testLogger.SetLevel(level)
			if testLogger.GetLevel() != tt.expected {
				t.Errorf("For arg '%s': expected logger level %v, got %v after SetLevel", tt.levelArg, tt.expected, testLogger.GetLevel())
			}
		})
	}
}

// TestLogOutput (existing test)
func TestLogOutput(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	originalOutput := testLogger.Out
	var buf bytes.Buffer
	testLogger.SetOutput(&buf)
	defer func() {
		testLogger.SetOutput(originalOutput)
	}()

	testLogger.SetLevel(logrus.DebugLevel)

	testMessage := "This is a test log message"

	logEntries := []struct{
		level logrus.Level
		logFunc func(...interface{})
		levelStr string
	} {
		{logrus.DebugLevel, testLogger.Debug, "debug"},
		{logrus.InfoLevel, testLogger.Info, "info"},
		{logrus.WarnLevel, testLogger.Warn, "warning"},
		{logrus.ErrorLevel, testLogger.Error, "error"},
	}

	for _, entry := range logEntries {
		entry.logFunc(testMessage)
		output := buf.String()
		if !strings.Contains(output, testMessage) {
			t.Errorf("%s message not found in output. Log output: %s", entry.levelStr, output)
		}
		expectedLevelStringInLog := entry.levelStr
		if entry.levelStr == "warn" {
			expectedLevelStringInLog = "warning"
		}
		if !strings.Contains(output, fmt.Sprintf("level=%s", expectedLevelStringInLog)){
			 t.Errorf("Log level string '%s' not found in %s message. Log output: %s", expectedLevelStringInLog, entry.levelStr, output)
		}
		buf.Reset()
	}
}

// TestLogLevelFromConfig (existing test)
func TestLogLevelFromConfig(t *testing.T) {
	tests := []struct {
		name              string
		configLogLevel    string
		expectedLevel     logrus.Level
		expectWarning     bool
		expectInfoDefault bool
	}{
		{"ValidLevelDebug", "debug", logrus.DebugLevel, false, false},
		{"ValidLevelInfo", "info", logrus.InfoLevel, false, false},
		{"ValidLevelWarn", "warn", logrus.WarnLevel, false, false},
		{"EmptyLogLevel", "", logrus.InfoLevel, false, true},
		{"InvalidLogLevel", "invalidvalue", logrus.InfoLevel, true, false},
		{"CaseInsensitiveDebug", "DEBUG", logrus.DebugLevel, false, false},
	}

	originalGlobalLogOut := log.Out
	originalGlobalLogLevel := log.GetLevel()
	defer func() {
		log.SetOutput(originalGlobalLogOut)
		log.SetLevel(originalGlobalLogLevel)
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)

			var resultingLevel logrus.Level
			var errParse error

			if tt.configLogLevel != "" {
				resultingLevel, errParse = logrus.ParseLevel(tt.configLogLevel)
				if errParse != nil {
					log.Warnf("Invalid LogLevel '%s' in config: %v. Defaulting to INFO.", tt.configLogLevel, errParse)
					log.SetLevel(logrus.InfoLevel)
				} else {
					log.SetLevel(resultingLevel)
				}
			} else {
				log.SetLevel(logrus.InfoLevel)
			}

			if log.GetLevel() != tt.expectedLevel {
				t.Errorf("Expected global log level %v, got %v for config input '%s'", tt.expectedLevel, log.GetLevel(), tt.configLogLevel)
			}

			logOutput := buf.String()
			if tt.expectWarning {
				if !strings.Contains(logOutput, "Invalid LogLevel") || !strings.Contains(logOutput, "Defaulting to INFO") {
					t.Errorf("Expected warning for invalid log level with input '%s', got: %s", tt.configLogLevel, logOutput)
				}
			}
		})
	}
}


// --- MockDiscordSession and helpers (existing) ---
type MockDiscordSession struct {
	*discordgo.Session
	CustomChannelMessageFunc func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error)
	TestStateOverride        *discordgo.State
}

func (m *MockDiscordSession) ChannelMessage(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
	if m.CustomChannelMessageFunc != nil {
		return m.CustomChannelMessageFunc(channelID, messageID, opts...)
	}
	if m.Session != nil {
		return m.Session.ChannelMessage(channelID, messageID, opts...)
	}
	return nil, fmt.Errorf("ChannelMessageFunc not implemented and no embedded session")
}

func (m *MockDiscordSession) State() *discordgo.State {
	if m.TestStateOverride != nil {
		return m.TestStateOverride
	}
	if m.Session != nil && m.Session.State != nil {
		return m.Session.State
	}
	st := &discordgo.State{}
	st.User = &discordgo.User{ID: "defaultMockBotID_in_State"}
	return st
}

func (m *MockDiscordSession) MessageReactionAdd(channelID, messageID, emojiID string, opts ...discordgo.RequestOption) error {
	log.Debugf("MockDiscordSession: MessageReactionAdd called with: chID=%s, msgID=%s, emoji=%s", channelID, messageID, emojiID)
	return nil
}

var (
	originalGlobalConfigForTest *Config
	testLogBufferForTest        *bytes.Buffer
)

func setupTestEnvironment() {
	originalGlobalConfigForTest = globalConfig
	testLogBufferForTest = new(bytes.Buffer)
	log.SetOutput(testLogBufferForTest)
	log.SetLevel(logrus.DebugLevel)
}

func teardownTestEnvironment() {
	globalConfig = originalGlobalConfigForTest
	log.SetOutput(os.Stderr)
	log.SetLevel(logrus.InfoLevel)
	testLogBufferForTest = nil
}

// TestMessageUpdateHandler (existing, modified for *discordgo.Message)
func TestMessageUpdateHandler(t *testing.T) {
	mockSess := &MockDiscordSession{Session: &discordgo.Session{}}

	testBotState := &discordgo.State{}
	testBotState.User = &discordgo.User{ID: "botTestID_in_HandlerTest"}
	mockSess.TestStateOverride = testBotState

	t.Run("IgnoreUpdateFromBot_M_Author", func(t *testing.T) {
		setupTestEnvironment()
		defer teardownTestEnvironment()
		mockSess.CustomChannelMessageFunc = nil

		update := &discordgo.MessageUpdate{
			Message: &discordgo.Message{
				ID:        "msg1",
				ChannelID: "ch1",
				Author:    &discordgo.User{ID: mockSess.State().User.ID},
			},
		}
		messageUpdateLogic(mockSess, update)
		output := testLogBufferForTest.String()
		expectedLog := "Ignoring message update: original message author is bot (m.Author.ID)"
		if !strings.Contains(output, expectedLog) {
			t.Errorf("Expected log '%s', got: %s", expectedLog, output)
		}
	})

	t.Run("IgnoreUpdateFromBot_FullMessage_Author", func(t *testing.T) {
		setupTestEnvironment()
		defer teardownTestEnvironment()
		mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
			return &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				Author:    &discordgo.User{ID: mockSess.State().User.ID},
				Content:   "test content",
			}, nil
		}
		updateEvent := &discordgo.MessageUpdate{
			Message: &discordgo.Message{
				ID:        "msg2",
				ChannelID: "ch1",
				Author:    &discordgo.User{ID: "userTestID"},
			},
		}
		messageUpdateLogic(mockSess, updateEvent)
		output := testLogBufferForTest.String()
		expectedLog := "Ignoring message update: full message author is bot (fullMessage.Author.ID)"
		if !strings.Contains(output, expectedLog) {
			t.Errorf("Expected log '%s', got: %s", expectedLog, output)
		}
	})

	baseMsgForPrioTest_Update := &discordgo.Message{ // Changed from baseMsgForPrioTest to avoid conflict
		ID:        "msgPrioUpdate",
		ChannelID: "chPrioUpdate",
		Author:    &discordgo.User{ID: "userPrioTestID"},
		Content:   "test content for priority in update",
	}

	ruleMatchingReaction_Update := func(emojiName string, priority int) Rule { // Changed from ruleMatchingReaction
		return Rule{
			Name: fmt.Sprintf("RuleFor%s_Update", emojiName),
			Actions: RuleActions{ReactionEmoji: emojiName, Priority: priority, PushoverDestination: "testdest"},
			Conditions: RuleConditions{ChannelID: "chPrioUpdate"},
		}
	}

	testsPreviouslyNotified_Update := []struct { // Changed from testsPreviouslyNotified
		name             string
		reactions        []*discordgo.MessageReactions
		rules            []Rule
		expectedPrioLog  string
	}{
		{
			name:      "Update_NoBotReactions",
			reactions: []*discordgo.MessageReactions{{Emoji: &discordgo.Emoji{Name: "👍"}, Me: false}},
			rules:     []Rule{ruleMatchingReaction_Update("👍", 0)},
			expectedPrioLog: fmt.Sprintf("Previously notified priority: %d", int(math.MaxInt32)),
		},
		{
			name:      "Update_BotReactionMatchesRule",
			reactions: []*discordgo.MessageReactions{{Emoji: &discordgo.Emoji{Name: "✅"}, Me: true}},
			rules:     []Rule{ruleMatchingReaction_Update("✅", 1)},
			expectedPrioLog: "Previously notified priority: 1",
		},
	}

	for _, tt := range testsPreviouslyNotified_Update {
		t.Run(tt.name, func(t *testing.T) {
			setupTestEnvironment()
			defer teardownTestEnvironment()

			currentMsg := *baseMsgForPrioTest_Update
			currentMsg.Reactions = tt.reactions

			mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				return &currentMsg, nil
			}
			updateEvent := &discordgo.MessageUpdate{Message: &currentMsg}
			globalConfig = &Config{Rules: tt.rules}
			messageUpdateLogic(mockSess, updateEvent)
			logOutput := testLogBufferForTest.String()
			processRulesLogStart := fmt.Sprintf("Processing rules for message ID %s", currentMsg.ID)
			if !strings.Contains(logOutput, processRulesLogStart) {
				t.Fatalf("ProcessRules log not found. Log: %s", logOutput)
			}
			if !strings.Contains(logOutput, tt.expectedPrioLog) {
				t.Errorf("Expected log '%s' to contain '%s'. Log: %s", processRulesLogStart, tt.expectedPrioLog, logOutput)
			}
		})
	}

	t.Run("ProcessValidUpdate_Calls_ProcessRules", func(t *testing.T) {
		setupTestEnvironment()
		defer teardownTestEnvironment()
		fetchedMessage := &discordgo.Message{
			ID:        "msg3", ChannelID: "ch1", Author: &discordgo.User{ID: "userTestID"},
			Content:   "new content", Reactions: []*discordgo.MessageReactions{},
		}
		mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
			if channelID == "ch1" && messageID == "msg3" { return fetchedMessage, nil }
			return nil, fmt.Errorf("unexpected ChannelMessage call: chID %s, msgID %s", channelID, messageID)
		}
		updateEvent := &discordgo.MessageUpdate{
			Message: &discordgo.Message{ID: "msg3", ChannelID: "ch1", Author: &discordgo.User{ID: "userTestID"}},
		}
		globalConfig = &Config{}
		messageUpdateLogic(mockSess, updateEvent)
		logOutput := testLogBufferForTest.String()
		expectedProcessRulesLog := fmt.Sprintf("Processing rules for message ID %s", fetchedMessage.ID)
		if !strings.Contains(logOutput, fmt.Sprintf("Received message update: ID=%s", fetchedMessage.ID)) {
			t.Errorf("Expected log ... Log: %s", logOutput)
		}
		if !strings.Contains(logOutput, fmt.Sprintf("Processing update for message: ID=%s", fetchedMessage.ID)) {
			t.Errorf("Expected log ... Log: %s", logOutput)
		}
		if !strings.Contains(logOutput, expectedProcessRulesLog) {
			t.Errorf("Expected ProcessRules log ... Log: %s", logOutput)
		}
		if !strings.Contains(logOutput, "Previously notified priority:") {
			t.Errorf("Expected ProcessRules log to contain 'Previously notified priority:'. Log: %s", logOutput)
		}
	})

	t.Run("ChannelMessageFetchError", func(t *testing.T) {
		setupTestEnvironment()
		defer teardownTestEnvironment()
		mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
			return nil, fmt.Errorf("simulated fetch error")
		}
		updateEvent := &discordgo.MessageUpdate{
			Message: &discordgo.Message{ID: "msg4", ChannelID: "ch1", Author: &discordgo.User{ID: "userTestID"}},
		}
		messageUpdateLogic(mockSess, updateEvent)
		output := testLogBufferForTest.String()
		if !strings.Contains(output, "Error fetching full message for update") {
			t.Errorf("Expected log message about fetch error, got: %s", output)
		}
		if strings.Contains(output, "Processing rules for message ID") {
			t.Errorf("ProcessRules should not have been called after fetch error, log: %s", output)
		}
	})
}

// --- New tests for messageReactionAddLogic ---
func TestMessageReactionAddHandler(t *testing.T) {
	mockSess := &MockDiscordSession{Session: &discordgo.Session{}}
	testBotState := &discordgo.State{}
	testBotState.User = &discordgo.User{ID: "botReactionTestID"}
	mockSess.TestStateOverride = testBotState

	baseReaction := &discordgo.MessageReactionAdd{
		MessageReaction: &discordgo.MessageReaction{
			UserID:    "userWhoReacted", // Default: not the bot
			MessageID: "msgReact",
			ChannelID: "chReact",
			Emoji:     discordgo.Emoji{Name: "👍"},
		},
	}

	// For ProcessRules call verification
	ruleForReactionTest := func(emojiName string, priority int) Rule {
		return Rule{
			Name: fmt.Sprintf("RuleForReact%s", emojiName),
			Actions: RuleActions{ReactionEmoji: emojiName, Priority: priority, PushoverDestination: "testdest"},
			Conditions: RuleConditions{ChannelID: "chReact"}, // Simple condition
		}
	}

	t.Run("IgnoreReactionFromBot", func(t *testing.T) {
		setupTestEnvironment()
		defer teardownTestEnvironment()

		// Create a specific reaction for this test where the bot is the author
		botReaction := &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				UserID:    mockSess.State().User.ID, // Bot is the one reacting
				MessageID: baseReaction.MessageID,    // Use other fields from base for consistency
				ChannelID: baseReaction.ChannelID,
				Emoji:     baseReaction.Emoji,
			},
		}

		messageReactionAddLogic(mockSess, botReaction)
		output := testLogBufferForTest.String()
		if !strings.Contains(output, "Ignoring reaction added by the bot itself") {
			t.Errorf("Expected log indicating bot's own reaction ignored, got: %s", output)
		}
	})

	t.Run("ChannelMessageFetchError_Reaction", func(t *testing.T) {
		setupTestEnvironment()
		defer teardownTestEnvironment()
		mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
			return nil, fmt.Errorf("simulated fetch error for reaction")
		}
		messageReactionAddLogic(mockSess, baseReaction)
		output := testLogBufferForTest.String()
		if !strings.Contains(output, "Error fetching full message for reaction add") {
			t.Errorf("Expected log for fetch error, got: %s", output)
		}
		if strings.Contains(output, "Processing rules for message ID") {
			t.Errorf("ProcessRules should not be called after fetch error. Log: %s", output)
		}
	})

	// Test cases for previouslyNotifiedRulePriority in messageReactionAddLogic
	msgForReactionPrioTest := &discordgo.Message{
		ID:        "msgReact", ChannelID: "chReact", Author:    &discordgo.User{ID: "originalAuthor"},
		Content:   "message content for reaction",
	}

	testsReactionPrio := []struct {
		name                  string
		messageReactionsOnFetch []*discordgo.MessageReactions // Reactions on the message when fetched
		rules                 []Rule
		expectedPrioLog       string
	}{
		{
			name:      "Reaction_NoBotReactionsOnMsg",
			messageReactionsOnFetch: []*discordgo.MessageReactions{{Emoji: &discordgo.Emoji{Name: "👍"}, Me: false}},
			rules:     []Rule{ruleForReactionTest("👍", 0)},
			expectedPrioLog: fmt.Sprintf("Previously notified priority: %d", int(math.MaxInt32)),
		},
		{
			name:      "Reaction_BotReactionMatchesRuleOnMsg",
			messageReactionsOnFetch: []*discordgo.MessageReactions{{Emoji: &discordgo.Emoji{Name: "✅"}, Me: true}}, // Bot already reacted with ✅
			rules:     []Rule{ruleForReactionTest("✅", 1)}, // Rule that would add ✅
			expectedPrioLog: "Previously notified priority: 1",
		},
	}

	for _, tt := range testsReactionPrio {
		t.Run(tt.name, func(t *testing.T) {
			setupTestEnvironment()
			defer teardownTestEnvironment()

			currentMsg := *msgForReactionPrioTest // copy
			currentMsg.Reactions = tt.messageReactionsOnFetch

			mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				return &currentMsg, nil
			}

			// The incoming reaction itself (baseReaction.Emoji) is what triggers this.
			// The previouslyNotifiedRulePriority is based on what's *already on the message*.
			globalConfig = &Config{Rules: tt.rules}

			messageReactionAddLogic(mockSess, baseReaction) // baseReaction has 👍 by a user
			logOutput := testLogBufferForTest.String()

			processRulesLogStart := fmt.Sprintf("Processing rules for message ID %s", currentMsg.ID)
			if !strings.Contains(logOutput, processRulesLogStart) {
				t.Fatalf("ProcessRules log not found for %s. Log: %s", tt.name, logOutput)
			}
			if !strings.Contains(logOutput, tt.expectedPrioLog) {
				t.Errorf("Test '%s': Expected log '%s' to contain '%s'. Log: %s", tt.name, processRulesLogStart, tt.expectedPrioLog, logOutput)
			}
		})
	}
}
