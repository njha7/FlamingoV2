package flamingoservice

import (
	"github.com/bwmarrin/discordgo"
)

const (
	// CommandPrefix is the prefix the bot listens for to identify commands.
	CommandPrefix string = "~"
)

var (
	// Commands is the source of truth for all available commands and command actions
	Commands = map[string][]string{
		"strike": {"", "super", "get", "clear", "help"},
		"stroke": {"", "status", "super", "get", "clear", "help"},
		"pasta":  {"get", "save", "edit", "list", "help"},
		"template":  {"get", "save", "edit", "list", "help"},
		"react":  {"get", "save", "delete", "list", "help"},
		"auth":   {"set", "delete", "test", "permissive", "list", "help"},
	}
)

// FlamingoService is an interface for services. Services are responsible for identifying a potential invocation.
// If a message is identified as a command, the service is responsible for replying.
type FlamingoService interface {
	IsCommand(message string) bool
	Handle(session *discordgo.Session, message *discordgo.Message)
}

// BooleanCommandSuccess is a wrapper for the return value of commands that return a boolean
type BooleanCommandSuccess struct {
	Command *discordgo.Message
	Result  bool
}

// ParseServiceResponse is a helper to remove some repetitive error handling boilerplate from code.
func ParseServiceResponse(session *discordgo.Session, channelID string, response interface{}, err error) {
	if err != nil {
		session.ChannelMessageSend(channelID, "An error occured. Please try again later.")
	} else {
		stringResponse, isString := response.(string)
		boolResponse, isBool := response.(BooleanCommandSuccess)
		embedResponse, isMessageEmbed := response.(*discordgo.MessageEmbed)
		if isString {
			session.ChannelMessageSend(channelID, stringResponse)
		}
		if isMessageEmbed {
			session.ChannelMessageSendEmbed(channelID, embedResponse)
		}
		if isBool {
			if boolResponse.Result {
				session.MessageReactionAdd(boolResponse.Command.ChannelID, boolResponse.Command.ID, "check")
			} else {
				session.MessageReactionAdd(boolResponse.Command.ChannelID, boolResponse.Command.ID, "X")
			}

		}
	}
}
