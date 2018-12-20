package flamingoservice

import (
	"github.com/bwmarrin/discordgo"
)

const (
	CommandPrefix string = "~"
)

type FlamingoService interface {
	IsCommand(message string) bool
	Handle(session *discordgo.Session, message *discordgo.Message)
}
