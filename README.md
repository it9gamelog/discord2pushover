# discord2pushover

## Overview

`discord2pushover` is a command-line application for Linux that monitors Discord messages in channels your bot is part of. Based on a flexible ruleset defined in a YAML configuration file, it can send notifications to [Pushover](https://pushover.net/). It's designed for foreground operation, making it suitable for integration with systemd or container environments.

Key functionalities include:
- Watching for new messages in Discord channels.
- Sending customizable Pushover notifications.
- Handling Pushover's emergency-priority notifications, including tracking acknowledgment status and reacting on Discord once acknowledged.
- Allowing emoji reactions on Discord messages when rules are matched.
- Supporting environment variable substitution within the configuration file for sensitive data.

## Features

- **Discord Message Monitoring**: Listens to messages in real-time.
- **Pushover Notifications**: Sends messages to specified Pushover users or groups.
- **Configurable Rules Engine**: Define detailed rules to filter messages based on:
    - Channel ID
    - Message reactions (emojis)
    - Bot @mentions
    - Specific user/role @mentions
    - Keywords in message content
- **Emergency Alert Handling**:
    - Supports Pushover's emergency priority (Priority 2).
    - Tracks acknowledgment status of emergency alerts via Pushover API.
    - Reacts on the original Discord message with a configurable emoji when an emergency alert is acknowledged.
    - Handles expiry of emergency alerts.
- **Discord Reactions**: Can automatically add a configurable emoji reaction to a Discord message when a rule matches.
- **Environment Variable Substitution**: Securely pass sensitive data (like tokens) into the configuration file using environment variables (e.g., `$DISCORD_TOKEN`, `${PUSHOVER_APP_KEY}`).
- **Graceful Shutdown**: Handles SIGINT/SIGTERM signals for clean shutdown.
- **Version Information**: Provides build version via `-version` flag.

## Configuration (`discord2pushover.yaml`)

The application looks for its configuration file in the following order:
1.  Path specified by the `-c` command-line flag (e.g., `-c /path/to/your/config.yaml`).
2.  `discord2pushover.yaml` in the current working directory.
3.  `discord2pushover.yml` in the current working directory.

If no configuration file is found, the application will print an error and exit.

### Global Settings

These settings are at the top level of your YAML file:

-   `discordToken`: (string, required) Your Discord Bot Token. **Important**: This must be a Bot token, not a user token. Example: `"YOUR_DISCORD_BOT_TOKEN"`
-   `pushoverAppKey`: (string, required) Your Pushover Application API Token. You need to register an application on the Pushover site to get this. Example: `"YOUR_PUSHOVER_APP_TOKEN"`
-   `logLevel`: (string, optional) Sets the application's logging level. Valid values are `"trace"`, `"debug"`, `"info"`, `"warn"`, `"error"`, `"fatal"`, and `"panic"`. If omitted or invalid, defaults to `"info"`. Example: `"debug"`

### Environment Variable Substitution

You can embed environment variables in your YAML configuration file. The application will replace placeholders like `"$VAR_NAME"` or `"${VAR_NAME}"` with the actual value of the `VAR_NAME` environment variable at startup. If an environment variable is not set, the placeholder string will remain as is (as of current implementation, though this might change to error out or use an empty string in strict mode later).

Example:
```yaml
discordToken: "$BOT_TOKEN"
pushoverAppKey: "${MY_PUSHOVER_APP_KEY}"
```

### Rules

The `rules` section is a list of rule objects. Rules are evaluated from top to bottom for each incoming Discord message. The first rule that matches all its conditions will have its actions triggered, and **no further rules will be processed for that message.**

Each `rule` object has the following structure:

-   `name`: (string, optional) A descriptive name for the rule. This is useful for logging and debugging.
    Example: `"Critical Error Alert"`
-   `conditions`: (object, required) An object defining the conditions that must ALL be met for this rule to trigger. If a condition field is omitted (e.g., `channelID` is not specified), that condition is considered to be met (i.e., it doesn't filter).
    -   `channelID`: (string, optional) The specific Discord channel ID to monitor. If omitted, the rule applies to messages from any channel the bot has access to.
        Example: `"123456789012345678"`
    -   `messageHasEmoji`: ([]string, optional) A list of emoji names (Unicode emoji character or custom emoji name without colons). The condition is met if the Discord message has a reaction with ANY of these emojis.
        Example: `["🔥", "alert_emoji"]`
    -   `reactToAtMention`: (boolean, optional) If `true`, the message must @mention the bot itself (either directly or via @everyone/@here). Defaults to `false` if omitted.
        Example: `true`
    -   `specificMentions`: ([]string, optional) A list of Discord User IDs or Role IDs. The condition is met if the message mentions ANY of these users or roles.
        Example: `["U123ABCDEFG", "R098ZYXWVU"]`
    -   `contentIncludes`: ([]string, optional) A list of keywords. ALL keywords in this list must be present in the message content for the condition to be met. The check is case-insensitive.
        Example: `["error", "database connection failed"]`
-   `actions`: (object, required) Defines the actions to take if all conditions are met.
    -   `pushoverDestination`: (string, required) The Pushover user key or group key to send the notification to.
        Example: `"uMyPushoverUserKey"` or `"gMyPushoverGroupKey"`
    -   `priority`: (integer, required) The Pushover notification priority. Valid values are:
        -   `-2`: Lowest
        -   `-1`: Low
        -   `0`: Normal
        -   `1`: High
        -   `2`: Emergency (requires `emergency` block below)
        Example: `1`
    -   `reactionEmoji`: (string, optional) A Unicode emoji or a custom Discord emoji name (without colons) to react with on the original Discord message.
        Example: `"✅"` or `"custom_reaction"`
    -   `emergency`: (object, optional) This block is **required if and only if `priority` is `2` (Emergency)**.
        -   `ackEmoji`: (string, required for emergency) The emoji to react with on the Discord message once the Pushover emergency notification has been acknowledged by a user.
            Example: `"👍"`
        -   `expire`: (integer, required for emergency) The Pushover `expire` parameter in seconds. This is the duration for which Pushover will keep trying to send the notification until it's acknowledged or expires. Maximum is 10800 seconds (3 hours), but Pushover recommends values up to 3600 (1 hour) for their retry/expire mechanism. This also dictates how long the bot will track the acknowledgement.
            Example: `3600` (1 hour)
        -   `retry`: (integer, required for emergency) The Pushover `retry` parameter in seconds. This defines how often Pushover should resend the notification within the `expire` period. Minimum is 30 seconds.
            Example: `60` (resend every 60 seconds)

### Example Configuration

```yaml
# Global settings
discordToken: "$DISCORD_BOT_TOKEN" # Replaced by environment variable DISCORD_BOT_TOKEN
pushoverAppKey: "${PUSHOVER_APP_KEY}" # Replaced by environment variable PUSHOVER_APP_KEY
logLevel: "info" # Optional: trace, debug, info, warn, error, fatal, panic

rules:
  - name: "Critical System Alert with Emoji"
    conditions:
      channelID: "123456789012345678" # Specific channel
      messageHasEmoji: ["🔥", "sos"] # If message has 🔥 OR sos reaction
      contentIncludes: ["critical", "system down"] # Must contain BOTH "critical" AND "system down"
    actions:
      pushoverDestination: "uYourPushoverUserKey"
      priority: 2 # Emergency
      reactionEmoji: "🚨" # Bot reacts with 🚨 on Discord
      emergency:
        ackEmoji: "✅" # Bot reacts with ✅ when Pushover alert is acknowledged
        expire: 3600 # Pushover will retry for 1 hour; bot tracks for this long.
        retry: 60    # Pushover retries every 60 seconds.

  - name: "Mentions for Support Team"
    conditions:
      specificMentions: ["R98765432109876543"] # Specific role mention
      # reactToAtMention: false (default)
    actions:
      pushoverDestination: "gSupportGroupKey" # Send to a Pushover group
      priority: 1 # High
      reactionEmoji: "🔔"

  - name: "DB Error in Any Channel"
    conditions:
      contentIncludes: ["database error"] # Message must contain "database error"
    actions:
      pushoverDestination: "uDBAdminUserKey"
      priority: 0 # Normal
      reactionEmoji: "💾"

  - name: "Bot Mention General"
    conditions:
      reactToAtMention: true # If bot is @mentioned
    actions:
      pushoverDestination: "uYourPushoverUserKey"
      priority: 0
      # No reactionEmoji for this rule
```

## Building

To build the application, ensure you have Go installed (version 1.18 or later recommended).

```bash
go build .
```
This will create a `discord2pushover` executable in the current directory.

## Running

Execute the binary, optionally providing a path to your configuration file:

```bash
./discord2pushover -c /path/to/your/discord2pushover.yaml
```

If the configuration file is named `discord2pushover.yaml` or `discord2pushover.yml` and located in the same directory as the executable, you can run it without the `-c` flag:

```bash
./discord2pushover
```

### Required Discord Bot Permissions

Ensure your Discord bot has the following permissions in the channels it needs to monitor and react in:
-   **View Channels** (also known as Read Messages)
-   **Read Message History**
-   **Add Reactions**

(Note: "Send Messages" permission is not strictly required for basic operation unless future features sending messages are added.)

## Signal Handling

The application listens for `SIGINT` (Ctrl+C) and `SIGTERM` signals. Upon receiving either of these, it will attempt to shut down gracefully by:
1.  Stopping the emergency acknowledgment poller.
2.  Closing the connection to Discord.
3.  Exiting.

## Version

To print the version information (version, commit hash, build date), use the `-version` flag:

```bash
./discord2pushover -version
```

This information is embedded at build time if building from a Git repository.

## Running with Docker

This application can be run as a Docker container. Images are automatically built and pushed to the GitHub Container Registry (GHCR).

### Prerequisites

- Docker installed on your system.
- A configuration file (`discord2pushover.yaml` or `discord2pushover.yml`) prepared as described in the [Configuration](#configuration-discord2pushoveryaml) section.

### Pulling the Image

You can pull the latest image using:

```bash
docker pull ghcr.io/it9gamelog/discord2pushover:latest
```

You can also pull images tagged with a specific commit SHA:

```bash
docker pull ghcr.io/it9gamelog/discord2pushover:[COMMIT_SHA]
```

### Running with `docker run`

To run the container, you need to mount your configuration file into the `/app` directory within the container. The application, by default (as per Dockerfile `WORKDIR /app` and `ENTRYPOINT`), will look for `discord2pushover.yaml` or `discord2pushover.yml` in its current working directory (`/app`).

**Example:**

Assuming your configuration file is located at `/path/to/your/discord2pushover.yaml` on your host machine:

```bash
docker run -d --name discord2pushover \
  -v /path/to/your/discord2pushover.yaml:/app/discord2pushover.yaml \
  --restart unless-stopped \
  ghcr.io/it9gamelog/discord2pushover:latest
```

**Explanation:**

- `-d`: Run the container in detached mode (in the background).
- `--name discord2pushover`: Assign a name to the container for easier management.
- `-v /path/to/your/discord2pushover.yaml:/app/discord2pushover.yaml`: Mounts your local configuration file to `/app/discord2pushover.yaml` inside the container. The application will automatically pick this up. If you name your file `discord2pushover.yml`, adjust the command accordingly: `-v /path/to/your/discord2pushover.yml:/app/discord2pushover.yml`.
- `--restart unless-stopped`: Configures the container to restart automatically unless it's manually stopped.
- `ghcr.io/it9gamelog/discord2pushover:latest`: The image to run.

If your configuration file has a different name or you want to place it in a different path inside the container, you can use the `-c` flag:

```bash
docker run -d --name discord2pushover \
  -v /path/to/your/custom_config.yaml:/config/custom_config.yaml \
  --restart unless-stopped \
  ghcr.io/it9gamelog/discord2pushover:latest -c /config/custom_config.yaml
```

Remember to replace `it9gamelog` with your actual GitHub username or organization name where this repository is hosted.

### Running with `docker-compose`

You can also use `docker-compose` for easier management. Create a `docker-compose.yml` file with the following content:

```yaml
services:
  discord2pushover:
    image: ghcr.io/it9gamelog/discord2pushover:latest
    container_name: discord2pushover
    restart: unless-stopped
    volumes:
      # Mount your configuration file.
      # Adjust the source path to where your config file is located on the host.
      # The target path should be /app/discord2pushover.yaml or /app/discord2pushover.yml
      # so the application can find it automatically.
      - ./discord2pushover.yaml:/app/discord2pushover.yaml
      # If you want to use a different config name/path inside the container,
      # you'll need to adjust the command:
      # - /path/to/your/custom_config.yaml:/config/custom_config.yaml
    # If you mounted the config to a non-default location (e.g., /config/custom_config.yaml),
    # you need to tell the application where to find it:
    # command: ["-c", "/config/custom_config.yaml"]
    # Environment variables for tokens can also be passed here if your config uses them.
    # environment:
    #   - DISCORD_BOT_TOKEN=your_actual_discord_token_here
    #   - PUSHOVER_APP_KEY=your_actual_pushover_app_key_here
```

To run with `docker-compose`:

1.  Save the content above as `docker-compose.yml` in a directory of your choice.
2.  Ensure your `discord2pushover.yaml` (or `.yml`) is correctly referenced in the `volumes` section.
3.  Navigate to that directory in your terminal and run:

```bash
docker-compose up -d
```

To stop and remove the container:

```bash
docker-compose down
```

This setup assumes your configuration file (`discord2pushover.yaml` or `discord2pushover.yml`) is present at the specified host path and will be mounted to `/app/` inside the container, where the application expects it by default.
