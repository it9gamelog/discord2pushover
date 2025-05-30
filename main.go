package main

import (
	"flag"
	"fmt" // Added for version printing
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"math" // Added for MaxInt32

	"github.com/gregdel/pushover"
	"github.com/sirupsen/logrus"
)

// globalConfig holds the loaded application configuration.
// It's used by various parts of the application, including event handlers.
var globalConfig *Config
var log = logrus.New()

// TrackedEmergencyMessage holds information about an emergency Pushover notification
// that requires acknowledgment tracking.
type TrackedEmergencyMessage struct {
	DiscordMessageID  string
	DiscordChannelID  string
	PushoverReceiptID string
	AckEmoji          string
	ExpiryTime        time.Time
}

// trackedMessages stores emergency messages that are pending acknowledgment.
// Keyed by PushoverReceiptID.
var trackedMessages sync.Map

var (
	// Populated by go build
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// DiscordSessionInterface defines the subset of discordgo.Session methods
// that our handlers use. This allows for easier mocking in tests.
type DiscordSessionInterface interface {
	ChannelMessage(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error)
	State() *discordgo.State // Provided by wrapper for *discordgo.Session
	MessageReactionAdd(channelID, messageID, emojiID string, opts ...discordgo.RequestOption) error
}

// DiscordGoSessionWrapper wraps a *discordgo.Session to satisfy DiscordSessionInterface.
type DiscordGoSessionWrapper struct {
	RealSession *discordgo.Session
}

// ChannelMessage calls the RealSession's ChannelMessage.
func (w *DiscordGoSessionWrapper) ChannelMessage(channelID, messageID string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
	return w.RealSession.ChannelMessage(channelID, messageID, opts...)
}

// State returns the RealSession's State.
func (w *DiscordGoSessionWrapper) State() *discordgo.State {
	if w.RealSession == nil { // Guard against nil RealSession
		return nil
	}
	return w.RealSession.State
}

// MessageReactionAdd calls the RealSession's MessageReactionAdd.
func (w *DiscordGoSessionWrapper) MessageReactionAdd(channelID, messageID, emojiID string, opts ...discordgo.RequestOption) error {
	return w.RealSession.MessageReactionAdd(channelID, messageID, emojiID, opts...)
}

// Ensure DiscordGoSessionWrapper satisfies DiscordSessionInterface at compile time.
var _ DiscordSessionInterface = &DiscordGoSessionWrapper{}


func main() {
	// Setup logging - initial minimal setup. Level will be set after config load.
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	// Default to InfoLevel, will be overridden by config if specified.
	log.SetLevel(logrus.InfoLevel)

	// App Info - Logged at default Info level for now.
	// If config sets a higher level (e.g. warn), this initial version log might not be seen
	// unless we parse log level from env/flag first, then from config.
	// For simplicity, we'll log version info after potential config-based level adjustment.
	// log.Infof("discord2pushover version %s, commit %s, built at %s", Version, Commit, Date)

	configPath := flag.String("c", "", "Path to the configuration file (e.g., discord2pushover.yaml)")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	// If version flag is set, print version and exit BEFORE config loading & full log setup.
	// Use fmt.Printf for this as log level isn't fully configured yet.
	if *versionFlag {
		fmt.Printf("discord2pushover version %s, commit %s, built at %s\n", Version, Commit, Date)
		os.Exit(0)
	}

	actualConfigPath := ""
	if *configPath != "" {
		if _, err := os.Stat(*configPath); err == nil {
			actualConfigPath = *configPath
		} else {
			log.Errorf("Config file specified by -c flag not found: %s", *configPath)
			os.Exit(1)
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			log.Errorf("Error getting current working directory: %v", err)
			os.Exit(1)
		}
		defaultPathYaml := filepath.Join(cwd, "discord2pushover.yaml")
		if _, err := os.Stat(defaultPathYaml); err == nil {
			actualConfigPath = defaultPathYaml
		} else {
			defaultPathYml := filepath.Join(cwd, "discord2pushover.yml")
			if _, err := os.Stat(defaultPathYml); err == nil {
				actualConfigPath = defaultPathYml
			}
		}
	}

	if actualConfigPath == "" {
		log.Error("Configuration file not found.")
		log.Error("Please specify a config file using the -c flag,")
		log.Error("or place 'discord2pushover.yaml' or 'discord2pushover.yml' in the current directory.")
		os.Exit(1)
	}

	log.Infof("Loading configuration from: %s", actualConfigPath)
	loadedConfig, err := LoadConfig(actualConfigPath) // Use a temporary variable
	if err != nil {
		// Use current log level (default Info) for this error, as config hasn't been processed for log level yet.
		log.Errorf("Error loading configuration: %v", err)
		os.Exit(1)
	}
	globalConfig = loadedConfig // Assign to the global variable

	// Now set log level from config
	if globalConfig.LogLevel != "" {
		parsedLevel, err := logrus.ParseLevel(globalConfig.LogLevel)
		if err != nil {
			log.Warnf("Invalid LogLevel '%s' in config: %v. Defaulting to INFO.", globalConfig.LogLevel, err)
			log.SetLevel(logrus.InfoLevel) // Default to Info on parse error
		} else {
			log.SetLevel(parsedLevel)
			log.Infof("Log level set to '%s' from configuration.", parsedLevel.String())
		}
	} else {
		log.Info("LogLevel not specified in config, using default: INFO.")
		// log.SetLevel(logrus.InfoLevel) // Already default, but explicit if needed
	}

	// Now log version info, as log level is configured.
	log.Infof("discord2pushover version %s, commit %s, built at %s", Version, Commit, Date)
	log.Info("Configuration loaded successfully.")


	if globalConfig.DiscordToken == "" {
		log.Error("DiscordToken is missing from the configuration.")
		os.Exit(1)
	}
	if globalConfig.PushoverAppKey == "" {
		log.Error("PushoverAppKey is missing from the configuration.")
		os.Exit(1)
	}
	// Note: PushoverUserKey (the destination) is per-rule, so not checked globally here.

	log.Info("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + globalConfig.DiscordToken)
	if err != nil {
		log.Errorf("Error creating Discord session: %v", err)
		os.Exit(1)
	}

	// Register handlers
	dg.AddHandler(messageCreate)
	dg.AddHandler(messageUpdate)

	// We need intents for messages and message reactions to get message update events with reaction data.
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		log.Errorf("Error opening connection to Discord: %v", err)
		os.Exit(1)
	}
	log.Info("Discord session opened successfully.")

	// Start polling for emergency acknowledgements
	go PollEmergencyAcknowledgements(dg, globalConfig) // Logging for poller start is inside the function

	log.Info("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	receivedSignal := <-sc
	log.Infof("Received signal: %v. Shutting down...", receivedSignal)

	// Cleanly close down the Discord session.
	log.Info("Closing Discord session...")
	err = dg.Close()
	if err != nil {
		log.Errorf("Error closing Discord session: %v", err)
	} else {
		log.Info("Discord session closed.")
	}
	log.Info("Exiting.")
}

// PollEmergencyAcknowledgements periodically checks Pushover for acknowledged emergency messages
// and reacts on Discord if they are acknowledged.
func PollEmergencyAcknowledgements(session *discordgo.Session, config *Config) {
	// Create a new Pushover app instance
	app := pushover.New(config.PushoverAppKey)

	if config == nil {
		log.Error("PollEmergencyAcknowledgements: globalConfig is nil, cannot poll.")
		return
	}
	if session == nil {
		log.Error("PollEmergencyAcknowledgements: Discord session is nil, cannot poll.")
		return
	}

	// How often to poll Pushover for receipt status
	// Requirement: "every 5 seconds"
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Info("Starting emergency acknowledgement polling (interval: 5s)...")

	for range ticker.C {
		trackedMessages.Range(func(key, value interface{}) bool {
			receiptID := key.(string)
			trackedMsg, ok := value.(TrackedEmergencyMessage)
			if !ok {
				log.Errorf("Error: Could not cast value for receipt %s to TrackedEmergencyMessage", receiptID)
				trackedMessages.Delete(receiptID)
				return true // continue iteration
			}

			// Check for expiry
			if time.Now().After(trackedMsg.ExpiryTime) {
				log.Infof("Emergency message (Receipt: %s, DiscordMsg: %s) expired without acknowledgement.",
					receiptID, trackedMsg.DiscordMessageID)
				trackedMessages.Delete(receiptID)
				return true // continue iteration
			}

			// Check Pushover for acknowledgment
			log.Debugf("Polling Pushover for receipt: %s (DiscordMsg: %s)", receiptID, trackedMsg.DiscordMessageID)

			receiptDetails, err := app.GetReceiptDetails(receiptID) // This is a blocking call, so it will wait for the response
			if err != nil {
				log.Errorf("Error checking Pushover receipt %s: %v", receiptID, err)
				// Don't remove from map, try again next time unless it's a permanent error (not handled yet)
			} else if receiptDetails.Status != 1 {
				log.Warnf("Pushover receipt %s returned non-success status (%d).", receiptID, receiptDetails.Status)
				// Remove from map
				trackedMessages.Delete(receiptID)
			} else if receiptDetails.Acknowledged {
				log.Infof("Pushover emergency message (Receipt: %s, DiscordMsg: %s) was acknowledged!",
					receiptID, trackedMsg.DiscordMessageID)

				if trackedMsg.AckEmoji != "" {
					errReact := session.MessageReactionAdd(trackedMsg.DiscordChannelID, trackedMsg.DiscordMessageID, trackedMsg.AckEmoji)
					if errReact != nil {
						log.Errorf("Error adding AckEmoji '%s' to Discord message %s (channel %s): %v",
							trackedMsg.AckEmoji, trackedMsg.DiscordMessageID, trackedMsg.DiscordChannelID, errReact)
					} else {
						log.Infof("Added AckEmoji '%s' to Discord message %s (channel %s).",
							trackedMsg.AckEmoji, trackedMsg.DiscordMessageID, trackedMsg.DiscordChannelID)
					}
				}
				trackedMessages.Delete(receiptID) // Remove from tracking
			} else {
				log.Debugf("Pushover receipt %s (DiscordMsg: %s) not yet acknowledged.", receiptID, trackedMsg.DiscordMessageID)
			}
			return true // continue iteration
		})
	}
}

// messageCreate will be called (by the discordgo library) every time a new
// message is created on any channel that the authenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Guard against nil State or User, which can happen in tests or edge cases.
	if s.State == nil || s.State.User == nil {
		log.Error("messageCreate: session state or user is nil. Cannot reliably determine bot ID. Skipping message.")
		return
	}
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Log the basic message info (can be removed or made more verbose later)
	log.Debugf("Received message: ID=%s, AuthorID=%s, ChannelID=%s, Content='%s'", m.ID, m.Author.ID, m.ChannelID, m.Content)

	// Process rules against the message
	if globalConfig != nil {
		wrapper := &DiscordGoSessionWrapper{RealSession: s}
		// For new messages, there's no prior notification context from bot reactions on this message event
		ProcessRules(m, globalConfig, wrapper, math.MaxInt32)
	} else {
		// This should ideally not happen if main() ensures globalConfig is initialized.
		log.Error("globalConfig is nil in messageCreate. Rules cannot be processed.")
	}
}

// messageUpdate will be called (by the discordgo library) every time a message is
// updated on any channel that the authenticated bot has access to.
// This includes changes to content, embeds, and reactions.
// This is the actual handler registered with DiscordGo.
func messageUpdate(s *discordgo.Session, m *discordgo.MessageUpdate) {
	wrapper := &DiscordGoSessionWrapper{RealSession: s}
	messageUpdateLogic(wrapper, m)
}

// messageUpdateLogic contains the actual logic for handling message updates.
// It accepts an interface to allow mocking for tests.
func messageUpdateLogic(s DiscordSessionInterface, m *discordgo.MessageUpdate) {
	currentSessionState := s.State()
	if currentSessionState == nil || currentSessionState.User == nil {
		log.Error("messageUpdateLogic: session state or user is nil. Cannot reliably determine bot ID. Skipping update.")
		return
	}
	botID := currentSessionState.User.ID

	// m.Author in MessageUpdate is the original message author.
	// If the original message was from the bot, ignore it.
	if m.Author != nil && m.Author.ID == botID {
		log.Debugf("Ignoring message update: original message author is bot (m.Author.ID) (MessageID: %s)", m.ID)
		return
	}

	log.Infof("Received message update: ID=%s, ChannelID=%s", m.ID, m.ChannelID)

	// m.Message might be incomplete, especially for reactions.
	// Fetch the full message to ensure all data (like reactions) is present.
	// No options are typically needed for just fetching a message by ID.
	fullMessage, err := s.ChannelMessage(m.ChannelID, m.ID)
	if err != nil {
		log.Errorf("Error fetching full message for update (ID: %s, ChannelID: %s): %v", m.ID, m.ChannelID, err)
		return
	}

	// Additional check: If the full message shows it was authored by the bot, ignore.
	if fullMessage.Author != nil && fullMessage.Author.ID == botID {
		log.Debugf("Ignoring message update: full message author is bot (fullMessage.Author.ID) (MessageID: %s)", fullMessage.ID)
		return
	}

	// Convert discordgo.Message to discordgo.MessageCreate so ProcessRules can be reused.
	// Note: This is a simplification. Some fields might not perfectly align or might be missing.
	// For ProcessRules, we primarily need ID, ChannelID, Content, Author, Mentions, Reactions, GuildID.
	// MessageCreate Author is *User, Message Author is *User.
	// MessageCreate GuildID is string, Message GuildID is string.
	// It's important that ProcessRules only accesses fields available and relevant in both.
	// A more robust solution might involve a shared interface or a dedicated processing function for updates.
	msgCreateLike := &discordgo.MessageCreate{
		Message: fullMessage,
	}

	// Log the basic message info
	log.Debugf("Processing update for message: ID=%s, AuthorID=%s, ChannelID=%s, Content='%s', Reactions: %d",
		fullMessage.ID, fullMessage.Author.ID, fullMessage.ChannelID, fullMessage.Content, len(fullMessage.Reactions))

	if globalConfig != nil {
		// Determine if a notification was likely sent by checking bot's reactions
		// against configured rule action emojis.
		previouslyNotifiedRulePriority := math.MaxInt32 // Higher value means lower Pushover priority

		if len(fullMessage.Reactions) > 0 && len(globalConfig.Rules) > 0 {
			for _, reaction := range fullMessage.Reactions {
				if reaction.Me { // Bot added this reaction
					for _, rule := range globalConfig.Rules {
						if rule.Actions.ReactionEmoji == reaction.Emoji.Name {
							// This reaction corresponds to a rule's action emoji.
							// Store the highest priority (lowest numerical value for Pushover).
							if rule.Actions.Priority < previouslyNotifiedRulePriority {
								previouslyNotifiedRulePriority = rule.Actions.Priority
							}
							// Log this finding for debugging
							log.Debugf("messageUpdateLogic: Bot reaction '%s' matches rule '%s' (Priority: %d). Current highest notified priority: %d",
								reaction.Emoji.Name, rule.Name, rule.Actions.Priority, previouslyNotifiedRulePriority)
						}
					}
				}
			}
		}
		if previouslyNotifiedRulePriority == math.MaxInt32 {
			log.Debugf("messageUpdateLogic: No prior bot reactions found matching rule actions.")
		} else {
			log.Debugf("messageUpdateLogic: Determined highest previously notified rule priority (from bot reactions) as: %d", previouslyNotifiedRulePriority)
		}

		ProcessRules(msgCreateLike, globalConfig, s, previouslyNotifiedRulePriority)
	} else {
		log.Error("globalConfig is nil in messageUpdate. Rules cannot be processed.")
	}
}
