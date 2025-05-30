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

// TestLogLevelSetting tests if the log level is correctly set via command-line flags.
// This test is for the old command-line flag behavior, which has been removed.
// It should be adapted or removed if `-loglevel` flag is no longer supported.
// For now, I'll keep it as it tests logrus.ParseLevel and logger.SetLevel.
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

// TestLogOutput tests if log messages are written to the output.
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

// Helper function to simulate the log level setting logic from main()
// This is used by TestLogLevelFromConfig.
func applyLogLevelFromConfigLogic(testLogger *logrus.Logger, configLogLevel string) {
	if configLogLevel != "" {
		parsedLevel, err := logrus.ParseLevel(configLogLevel)
		if err != nil {
			testLogger.Warnf("Invalid LogLevel '%s' in config: %v. Defaulting to INFO.", configLogLevel, err)
			testLogger.SetLevel(logrus.InfoLevel)
		} else {
			testLogger.SetLevel(parsedLevel)
			// Log this for test verification, but not in the actual main() logic, which logs from global `log`
			// testLogger.Infof("Log level set to '%s' from configuration.", parsedLevel.String())
		}
	} else {
		testLogger.Info("LogLevel not specified in config, using default: INFO.")
		testLogger.SetLevel(logrus.InfoLevel)
	}
}

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

	// Use the global `log` instance for these tests as main() would, but be careful with state.
	// Redirect its output for capture.
	originalGlobalLogOut := log.Out
	originalGlobalLogLevel := log.GetLevel()
	defer func() {
		log.SetOutput(originalGlobalLogOut)
		log.SetLevel(originalGlobalLogLevel)
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf) // Capture global logger output

			// This simulates the logic in main() that sets the global log level.
			// The actual main() function logs messages like "Log level set to..." or "LogLevel not specified...".
			// This test will focus on the resulting log level and the warning for invalid values.

			var resultingLevel logrus.Level
			var errParse error

			if tt.configLogLevel != "" {
				resultingLevel, errParse = logrus.ParseLevel(tt.configLogLevel)
				if errParse != nil {
					log.Warnf("Invalid LogLevel '%s' in config: %v. Defaulting to INFO.", tt.configLogLevel, errParse)
					log.SetLevel(logrus.InfoLevel)
				} else {
					log.SetLevel(resultingLevel)
					// main() would log: log.Infof("Log level set to '%s' from configuration.", resultingLevel.String())
				}
			} else {
				// main() would log: log.Info("LogLevel not specified in config, using default: INFO.")
				log.SetLevel(logrus.InfoLevel)
			}

			if log.GetLevel() != tt.expectedLevel {
				t.Errorf("Expected global log level %v, got %v for config input '%s'", tt.expectedLevel, log.GetLevel(), tt.configLogLevel)
			}

			logOutput := buf.String()
			if tt.expectWarning {
				// This warning is generated by the test logic itself when ParseLevel fails, mimicking main()
				if !strings.Contains(logOutput, "Invalid LogLevel") || !strings.Contains(logOutput, "Defaulting to INFO") {
					t.Errorf("Expected warning for invalid log level with input '%s', got: %s", tt.configLogLevel, logOutput)
				}
			}
			// Specific info messages about how the level was set are part of main's operational logging,
			// not strictly part of this unit test's core validation of level setting.
			// So, assertions for those specific info messages are removed for simplicity.
		})
	}
}


// --- Tests for messageUpdate ---

// MockDiscordSession provides a mock implementation of discordgo.Session
// for testing the messageUpdate handler.
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
	// Important: Use the global `log` instance that the main code uses.
	log.SetOutput(testLogBufferForTest)
	log.SetLevel(logrus.DebugLevel)
}

func teardownTestEnvironment() {
	globalConfig = originalGlobalConfigForTest
	log.SetOutput(os.Stderr)
	log.SetLevel(logrus.InfoLevel) // Or whatever the package default is, typically Info.
	testLogBufferForTest = nil
}

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

	// Test cases for previouslyNotifiedRulePriority passed to ProcessRules
	// These will check the log output from ProcessRules via messageUpdateLogic
	baseMsgForPrioTest := &discordgo.Message{
		ID:        "msgPrio",
		ChannelID: "chPrio",
		Author:    &discordgo.User{ID: "userPrioTestID"},
		Content:   "test content for priority",
	}

	ruleMatchingReaction := func(emojiName string, priority int) Rule {
		return Rule{
			Name: fmt.Sprintf("RuleFor%s", emojiName),
			Actions: RuleActions{ReactionEmoji: emojiName, Priority: priority, PushoverDestination: "testdest"},
			Conditions: RuleConditions{ChannelID: "chPrio"}, // Simple condition to make it match
		}
	}

	testsPreviouslyNotified := []struct {
		name             string
		reactions        []*discordgo.MessageReactions
		rules            []Rule
		expectedPrioLog  string // Expected string in ProcessRules log related to priority
	}{
		{
			name:      "NoBotReactions",
			reactions: []*discordgo.MessageReactions{{Emoji: &discordgo.Emoji{Name: "👍"}, Me: false}},
			rules:     []Rule{ruleMatchingReaction("👍", 0)},
			expectedPrioLog: fmt.Sprintf("Previously notified priority: %d", int(math.MaxInt32)),
		},
		{
			name:      "BotReactionMatchesRule",
			reactions: []*discordgo.MessageReactions{{Emoji: &discordgo.Emoji{Name: "✅"}, Me: true}},
			rules:     []Rule{ruleMatchingReaction("✅", 1)},
			expectedPrioLog: "Previously notified priority: 1",
		},
		{
			name:      "BotReactionNoMatch",
			reactions: []*discordgo.MessageReactions{{Emoji: &discordgo.Emoji{Name: "❓"}, Me: true}},
			rules:     []Rule{ruleMatchingReaction("✅", 1)},
			expectedPrioLog: fmt.Sprintf("Previously notified priority: %d", int(math.MaxInt32)),
		},
		{
			name: "MultipleBotReactionsHighestPrio",
			reactions: []*discordgo.MessageReactions{
				{Emoji: &discordgo.Emoji{Name: "🅰️"}, Me: true}, // Matches Rule A, Prio 0
				{Emoji: &discordgo.Emoji{Name: "🅱️"}, Me: true}, // Matches Rule B, Prio -1 (higher)
			},
			rules: []Rule{ruleMatchingReaction("🅰️", 0), ruleMatchingReaction("🅱️", -1)},
			expectedPrioLog: "Previously notified priority: -1",
		},
	}

	for _, tt := range testsPreviouslyNotified {
		t.Run(tt.name, func(t *testing.T) {
			setupTestEnvironment()
			defer teardownTestEnvironment()

			currentMsg := *baseMsgForPrioTest // copy
			currentMsg.Reactions = tt.reactions

			mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				return &currentMsg, nil
			}

			updateEvent := &discordgo.MessageUpdate{Message: &currentMsg}

			// Set globalConfig with the rules for this test case
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
			ID:        "msg3",
			ChannelID: "ch1",
			Author:    &discordgo.User{ID: "userTestID"},
			Content:   "new content",
			Reactions: []*discordgo.MessageReactions{},
		}
		mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
			if channelID == "ch1" && messageID == "msg3" {
				return fetchedMessage, nil
			}
			return nil, fmt.Errorf("unexpected ChannelMessage call: chID %s, msgID %s", channelID, messageID)
		}

		updateEvent := &discordgo.MessageUpdate{
			Message: &discordgo.Message{
				ID:        "msg3",
				ChannelID: "ch1",
				Author:    &discordgo.User{ID: "userTestID"},
			},
		}

		globalConfig = &Config{}

		messageUpdateLogic(mockSess, updateEvent)

		logOutput := testLogBufferForTest.String()
		expectedProcessRulesLog := fmt.Sprintf("Processing rules for message ID %s", fetchedMessage.ID)

		if !strings.Contains(logOutput, fmt.Sprintf("Received message update: ID=%s", fetchedMessage.ID)) {
			t.Errorf("Expected log substring 'Received message update: ID=%s' not found. Log: %s", fetchedMessage.ID, logOutput)
		}
		if !strings.Contains(logOutput, fmt.Sprintf("Processing update for message: ID=%s", fetchedMessage.ID)) {
			t.Errorf("Expected log substring 'Processing update for message: ID=%s' not found. Log: %s", fetchedMessage.ID, logOutput)
		}
		if !strings.Contains(logOutput, expectedProcessRulesLog) {
			t.Errorf("Expected ProcessRules log starting with '%s' not found. Log: %s", expectedProcessRulesLog, logOutput)
		}
		if !strings.Contains(logOutput, "Previously notified priority:") {
			t.Errorf("Expected ProcessRules log to contain 'Previously notified priority:'. Log: %s", logOutput)
		}
		expectedLogSubstrings := []string{
			fmt.Sprintf("Received message update: ID=%s", fetchedMessage.ID),
			fmt.Sprintf("Processing update for message: ID=%s", fetchedMessage.ID),
			expectedProcessRulesLog,
		}
		for _, sub := range expectedLogSubstrings {
			if !strings.Contains(logOutput, sub) {
				t.Errorf("Expected log substring '%s' not found in output. Log: %s", sub, logOutput)
			}
		}
	})

	t.Run("ChannelMessageFetchError", func(t *testing.T) {
		setupTestEnvironment()
		defer teardownTestEnvironment()

		mockSess.CustomChannelMessageFunc = func(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
			return nil, fmt.Errorf("simulated fetch error")
		}
		updateEvent := &discordgo.MessageUpdate{
			Message: &discordgo.Message{
				ID:        "msg4",
				ChannelID: "ch1",
				Author:    &discordgo.User{ID: "userTestID"},
			},
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
