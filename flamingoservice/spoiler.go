package flamingoservice

import (
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	spoilerServiceName = "Spoil"
)

var (
	spoiler, _ = regexp.Compile(`\|\|.*\|\|`)
)

// SpoilerClient is responsible for auto-decoding spoilers
type SpoilerClient struct {
	AuthClient *AuthClient
}

// NewSpoilerClient constructs a SpoilerClient
func NewSpoilerClient(authClient *AuthClient) *SpoilerClient {
	return &SpoilerClient{
		AuthClient: authClient,
	}
}

// IsCommand identifies a message as a potential command
func (spoilerClient *SpoilerClient) IsCommand(message string) bool {
	return spoiler.MatchString(message)
}

// Handle parses a command message and performs the commanded action
func (spoilerClient *SpoilerClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	if !spoilerClient.AuthClient.Authorize(message.GuildID, message.Author.ID, "spoiler", "") {
		contents := strings.Replace(message.Content, "||", "", -1)
		ParseServiceResponse(session, message.ChannelID, contents, nil)
	}
}
