package main

import (
	"fmt" // Keep fmt for error wrapping
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	DiscordToken   string `yaml:"discordToken"`
	PushoverAppKey string `yaml:"pushoverAppKey"`
	LogLevel       string `yaml:"logLevel,omitempty"` // Added LogLevel
	Rules          []Rule `yaml:"rules"`
}

// Rule defines a single rule for processing messages.
type Rule struct {
	Name       string         `yaml:"name"`
	Conditions RuleConditions `yaml:"conditions"`
	Actions    RuleActions    `yaml:"actions"`
}

// RuleConditions defines the conditions for a rule to match.
type RuleConditions struct {
	ChannelID        string   `yaml:"channelId"`
	MessageHasEmoji  []string `yaml:"messageHasEmoji"`
	ReactToAtMention bool     `yaml:"reactToAtMention"`
	SpecificMentions []string `yaml:"specificMentions"`
	ContentIncludes  []string `yaml:"contentIncludes"`
}

// RuleActions defines the actions to take when a rule matches.
type RuleActions struct {
	PushoverDestination string           `yaml:"pushoverDestination"`
	Priority            int              `yaml:"priority"`
	ReactionEmoji       string           `yaml:"reactionEmoji"`
	Emergency           *EmergencyParams `yaml:"emergency,omitempty"`
}

// EmergencyParams defines parameters for Pushover emergency priority messages.
type EmergencyParams struct {
	AckEmoji string `yaml:"ackEmoji"`
	Expire   int    `yaml:"expire"`
	Retry    int    `yaml:"retry"`
}

// LoadConfig reads a YAML file from filePath, parses it into a Config struct,
// and replaces environment variable placeholders.
func LoadConfig(filePath string) (*Config, error) {
	// Read the YAML file
	log.Infof("Reading configuration file: %s", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	// Substitute environment variables
	substitutedData := substituteEnvVars(data)

	// Parse the YAML
	var cfg Config
	log.Info("Parsing YAML configuration...")
	err = yaml.Unmarshal(substitutedData, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filePath, err)
	}
	log.Info("YAML configuration parsed successfully.")
	return &cfg, nil
}

// substituteEnvVars replaces placeholders like $VAR_NAME or ${VAR_NAME} in the
// input byte slice with corresponding environment variable values.
func substituteEnvVars(data []byte) []byte {
	s := string(data)
	// Regex to find $VAR_NAME or ${VAR_NAME}
	// It captures VAR_NAME in both cases
	r := regexp.MustCompile(`\$(\{([A-Z_][A-Z0-9_]*)\}|([A-Z_][A-Z0-9_]*))`)

	replacedString := r.ReplaceAllStringFunc(s, func(found string) string {
		var varName string
		if strings.HasPrefix(found, "${") && strings.HasSuffix(found, "}") {
			varName = found[2 : len(found)-1]
		} else {
			varName = found[1:]
		}

		val, isSet := os.LookupEnv(varName)
		if isSet {
			log.Debugf("Substituting environment variable '%s' with value (length %d).", varName, len(val))
			// For security/privacy, don't log the actual value if it could be sensitive.
			// If you need to debug specific values, you can temporarily log `val` itself.
		} else {
			log.Debugf("Environment variable '%s' not set. Placeholder '%s' will remain.", varName, found)
			return found // Leave placeholder if not set
		}
		return val
	})
	return []byte(replacedString)
}
