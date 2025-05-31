Google Jules Prompt Used

# Task 1 - Codes Built From Zero

```
I want to create a Go CLI Project that does watch for Discord message, and if certain conditions are matched (mentioned below), send an Pushover notification to a preconfigured group. And also checks for Pushover recepit for emergency level notification, and create an emoticon on that particular message.

# Overview

This application shall be named discord2pushover. It should be runnable in Linux. It is expected to run in command line in foreground without forking, and exit nicely upon SIGINT and SIGTERM, such that could be easily integrated with systemctl or embedded as a container.

# Execution Mode

It should also accept one yaml configuration file. The app should look for that file in a path specificed by -c argument, or discord2pushover.yaml or discord2pushover.yml in the working directory, in that order mentioned.

The configuration schema is to be designed by you. This should be a place to contains the API keys for Discord and Pushover and alike, as well as the matching rules. Value "$FOO_BAR" shall be replaced by Environmental variables FOO_BAR, to allow user to use ENV for certain values if they want to.

# Configuration File and Rules

Here is the important part - the rules. Each rule has a condition and action part, and rules shall be evaluated from top to bottom and stop evaluating as soon as one action is hit. Condition that could be specified:

1. Discord channel ID
2. Message emoji (i.e. if any User put an emoji on the message)
3. Boolean: react to @mention or not
4. Message content includes (if the message has certain keywords) The conditions shall be evaluated in "AND"

Action that could be specified:

1. The Pushover destination
2. Priority level
3. The emoji that the app shall put on the message, once the condition is satisfy and action is taken
4. If it's an emergency level, the emoji that the app shall put on the message when it's acknowledged
5. If it's an emergency level, the expire and repeat parameter for that

# Others

1. The app should need no persistent storage. It needs not to react to any discord messages that were sent when the app is not running
2. The app need not to react to emergency ack that it didn't remember
3. The app shall tracked for ack of emergency level for 1 hours at most, and every 5 seconds.
```

Time Used: 20 mins. Commit: [9b3916284ac5ea8da05bfab635c89ae7fd017e08](../../commit/9b3916284ac5ea8da05bfab635c89ae7fd017e08)

# Task 2 - Achieving Main Goal

The code could not be built without minor tweak. Afterwards, I found that it wasn't listening for Message Update but only Message Create.

As I made changes and committed the code,  I could not resume from the same task.

Interestingly, Task 2 actually verify the build with `go build .`, I didn't know it could do that. And also fill in a lot unit-test, which I didn't ask it to do.

## Prompt 1

```
A few problem to be fixed

# React to message update

However, there is a major issue. As all emoji reaction to a message is only added after the message is sent, so there will never be any emoji reaction at message create, and the rules will never hit.

Modify the code such that it watches all the message updates, and check the updates aginst the rules.
ReactToAtMention shall be specified at all times. It should be used for the program to differentiate if pushover notification has been sent, and if so, not sending again.
When checking for ReactToAtMention, remember check the "Me" property that user marking ReactToAtMention will not affect the program logics.

# Verbosity

The program is too noisy. Please adopt logrus (or something else capable) as logging mechanism.

Change all console log statement to use the new logging mechanism instead
Determine the apporiate level to use. DEBUG for per rules evaluation, INFO for each incoming message, ERROR for error. You get the idea.
add one more global config for user to adjust the verbosity. Default to INFO or above.
```

Time Used: 10 mins. No commit.

## Human Intervention

I reviewed the code in Jules and found that it still doesn't understand the idempotence requirement, so prompt it to modify again.

## Prompt 2

```
# React to message updates
If an notification has ever been sent, do not consider for processing again. My suggestion is check if ReactToAtMention appears in the message, if so, stop rule processing (also the rule processing further down). A caveat is that if a rule of lower priority (in the tail of the config) is sent, a rule of higher prioritized rule can also hit after update, which I think is actually good.

# Logging
Please use the config file for user to tune the log level, not argument flag.
```

Time used: 10 mins. Commit: [21dca61440f61f13a77de56c169051177516d29c](../../commit/21dca61440f61f13a77de56c169051177516d29c)

## Human Intervention

I checked out the code, and found that Message Update handler does not receive any 

## Prompt 3

```
Also handle MessageReactionAdd event, such that any reaction added by user could trigger the rule processing. Beware to filter out MessageReactionAdd event triggered by the bot itself.

For conditions, let me clarify that if ANY emojis of MessageHasEmoji match, it is considered fulfilled for the MessageHasEmoji condition. It does not have to be all emojis.
```

Time Used: 10 mins. Commit: [2a546d90d2ac7b636fb00c6bdadb813e07696d23](../../commit/2a546d90d2ac7b636fb00c6bdadb813e07696d23)

# Task 3 - CI/CD

I am satisfied with the result, except it got the priority part wrong which I fixed in [fd84256bf0678e228479376dd2483fd98035eabf](../../commit/fd84256bf0678e228479376dd2483fd98035eabf).

As a final touch, I need it to be production grade, so CI/CD that is.

```
Create CI/CD which runs on Github

Deliverable:

CI: test and build a container for amd64
CD: publish the container (i think it could be stored in github, right? or do I need any other registry?)
modify the README accordingly for how to run such container, with CLI and docker compose instruction. Don't forget ansfk linking the configurration file
```

Time Used: 7 mins. Commit: [2b5f6fd8dc8e1e6e30bb91c6b78e88e11acb9fd3](../../commit/2b5f6fd8dc8e1e6e30bb91c6b78e88e11acb9fd3)

There were a few minor mistakes prevented the pipeline to run in just one shot. A few fixes were needed.
