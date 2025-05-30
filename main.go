package main

import (
	"flag"
	"fmt"
	"log" // Added for more consistent logging
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gregdel/pushover"
)

// globalConfig holds the loaded application configuration.
// It's used by various parts of the application, including event handlers.
var globalConfig *Config

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

func main() {
	fmt.Printf("discord2pushover version %s, commit %s, built at %s\n", Version, Commit, Date)

	configPath := flag.String("c", "", "Path to the configuration file (e.g., discord2pushover.yaml)")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		// Version info already printed
		os.Exit(0)
	}

	actualConfigPath := ""
	if *configPath != "" {
		if _, err := os.Stat(*configPath); err == nil {
			actualConfigPath = *configPath
		} else {
			fmt.Printf("Error: Config file specified by -c flag not found: %s\n", *configPath)
			os.Exit(1)
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error getting current working directory: %v\n", err)
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
		fmt.Println("Error: Configuration file not found.")
		fmt.Println("Please specify a config file using the -c flag,")
		fmt.Println("or place 'discord2pushover.yaml' or 'discord2pushover.yml' in the current directory.")
		os.Exit(1)
	}

	fmt.Printf("Loading configuration from: %s\n", actualConfigPath)
	loadedConfig, err := LoadConfig(actualConfigPath) // Use a temporary variable
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}
	globalConfig = loadedConfig // Assign to the global variable

	fmt.Printf("Configuration loaded successfully.\n") // Simplified output

	if globalConfig.DiscordToken == "" {
		fmt.Println("Error: DiscordToken is missing from the configuration.")
		os.Exit(1)
	}
	if globalConfig.PushoverAppKey == "" {
		fmt.Println("Error: PushoverAppKey is missing from the configuration.")
		os.Exit(1)
	}
	// Note: PushoverUserKey (the destination) is per-rule, so not checked globally here.

	log.Println("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + globalConfig.DiscordToken)
	if err != nil {
		log.Printf("Error creating Discord session: %v\n", err)
		os.Exit(1)
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate)

	// We only care about receiving message events.
	dg.Identify.Intents = discordgo.IntentsGuildMessages

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		log.Printf("Error opening connection to Discord: %v\n", err)
		os.Exit(1)
	}
	log.Println("Discord session opened successfully.")

	// Start polling for emergency acknowledgements
	go PollEmergencyAcknowledgements(dg, globalConfig) // Logging for poller start is inside the function

	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	receivedSignal := <-sc
	log.Printf("Received signal: %v. Shutting down...", receivedSignal)

	// Cleanly close down the Discord session.
	log.Println("Closing Discord session...")
	err = dg.Close()
	if err != nil {
		log.Printf("Error closing Discord session: %v", err)
	} else {
		log.Println("Discord session closed.")
	}
	log.Println("Exiting.")
}

// PollEmergencyAcknowledgements periodically checks Pushover for acknowledged emergency messages
// and reacts on Discord if they are acknowledged.
func PollEmergencyAcknowledgements(session *discordgo.Session, config *Config) {
	// Create a new Pushover app instance
	app := pushover.New(config.PushoverAppKey)

	if config == nil {
		log.Println("PollEmergencyAcknowledgements: globalConfig is nil, cannot poll.")
		return
	}
	if session == nil {
		log.Println("PollEmergencyAcknowledgements: Discord session is nil, cannot poll.")
		return
	}

	// How often to poll Pushover for receipt status
	// Requirement: "every 5 seconds"
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Println("Starting emergency acknowledgement polling (interval: 5s)...")

	for range ticker.C {
		trackedMessages.Range(func(key, value interface{}) bool {
			receiptID := key.(string)
			trackedMsg, ok := value.(TrackedEmergencyMessage)
			if !ok {
				log.Printf("Error: Could not cast value for receipt %s to TrackedEmergencyMessage", receiptID)
				trackedMessages.Delete(receiptID)
				return true // continue iteration
			}

			// Check for expiry
			if time.Now().After(trackedMsg.ExpiryTime) {
				log.Printf("Emergency message (Receipt: %s, DiscordMsg: %s) expired without acknowledgement.",
					receiptID, trackedMsg.DiscordMessageID)
				trackedMessages.Delete(receiptID)
				return true // continue iteration
			}

			// Check Pushover for acknowledgment
			log.Printf("Polling Pushover for receipt: %s (DiscordMsg: %s)", receiptID, trackedMsg.DiscordMessageID)

			receiptDetails, err := app.GetReceiptDetails(receiptID) // This is a blocking call, so it will wait for the response
			if err != nil {
				log.Printf("Error checking Pushover receipt %s: %v", receiptID, err)
				// Don't remove from map, try again next time unless it's a permanent error (not handled yet)
			} else if receiptDetails.Status != 1 {
				log.Printf("Pushover receipt %s returned non-success status (%d).", receiptID, receiptDetails.Status)
				// Remove from map
				trackedMessages.Delete(receiptID)
			} else if receiptDetails.Acknowledged {
				log.Printf("Pushover emergency message (Receipt: %s, DiscordMsg: %s) was acknowledged!",
					receiptID, trackedMsg.DiscordMessageID)

				if trackedMsg.AckEmoji != "" {
					errReact := session.MessageReactionAdd(trackedMsg.DiscordChannelID, trackedMsg.DiscordMessageID, trackedMsg.AckEmoji)
					if errReact != nil {
						log.Printf("Error adding AckEmoji '%s' to Discord message %s (channel %s): %v",
							trackedMsg.AckEmoji, trackedMsg.DiscordMessageID, trackedMsg.DiscordChannelID, errReact)
					} else {
						log.Printf("Added AckEmoji '%s' to Discord message %s (channel %s).",
							trackedMsg.AckEmoji, trackedMsg.DiscordMessageID, trackedMsg.DiscordChannelID)
					}
				}
				trackedMessages.Delete(receiptID) // Remove from tracking
			} else {
				log.Printf("Pushover receipt %s (DiscordMsg: %s) not yet acknowledged.", receiptID, trackedMsg.DiscordMessageID)
			}
			return true // continue iteration
		})
	}
}

// messageCreate will be called (by the discordgo library) every time a new
// message is created on any channel that the authenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Log the basic message info (can be removed or made more verbose later)
	// fmt.Printf("Received message: ID=%s, AuthorID=%s, ChannelID=%s, Content='%s'\n", m.ID, m.Author.ID, m.ChannelID, m.Content)

	// Process rules against the message
	if globalConfig != nil {
		ProcessRules(m, globalConfig, s)
	} else {
		// This should ideally not happen if main() ensures globalConfig is initialized.
		fmt.Println("Error: globalConfig is nil in messageCreate. Rules cannot be processed.")
	}
}
