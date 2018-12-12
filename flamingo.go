package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/bwmarrin/discordgo"
)

const (
	commandPrefix string = "~"
	bucket        string = "flamingo-bot"
	region        string = "us-east-1"
)

var (
	DISCORD_TOKEN, AWS_ACCESS_KEY, AWS_SECRET_KEY string
	local                                         bool
	flamingoLogger                                *log.Logger
	flamingoErrLogger                             *log.Logger
	strikeService                                 *StrikeClient
	pastaService                                  *PastaClient
)

func init() {
	flamingoLogger = BuildServiceLogger("Flamingo")
	flamingoErrLogger = BuildServiceErrorLogger("Flamingo")
	//Dumb and lazy hack
	flag.BoolVar(&local, "local", false, "Flag for running waimote in local test mode.")
	flag.StringVar(&DISCORD_TOKEN, "t", "", "Discord bot token.")
	flag.StringVar(&AWS_ACCESS_KEY, "ak", "", "AWS Access Key")
	flag.StringVar(&AWS_SECRET_KEY, "sk", "", "AWS Secret Key")
	flag.Parse()
	if local {
		flamingoLogger.Println("Running locally")
		//Running locally, pass creds as flags
	} else {
		//Run with creds in environment
		flamingoLogger.Println("Running remotely")
		DISCORD_TOKEN = os.Getenv("DISCORD_TOKEN")
		AWS_ACCESS_KEY = os.Getenv("AWS_ACCESS_KEY")
		AWS_SECRET_KEY = os.Getenv("AWS_SECRET_KEY")
	}
}

func main() {
	discord, err := discordgo.New("Bot " + DISCORD_TOKEN)
	if err != nil {
		flamingoErrLogger.Println("Error creating Discord session: ", err)
		return
	}
	//Initialize services before starting Flamingo
	//AWS service client construction
	awsSess := session.Must(session.NewSession(
		aws.NewConfig().
			WithCredentials(credentials.NewStaticCredentials(AWS_ACCESS_KEY, AWS_SECRET_KEY, "")).
			WithMaxRetries(3),
	))
	ddb := dynamodb.New(awsSess, aws.NewConfig().WithRegion(region))
	//Flamingo service Client construction
	strikeService = NewStrikeClient(discord, ddb)
	pastaService = NewPastaClient(discord, ddb)
	//Start Flamingo
	err = discord.Open()
	if err != nil {
		flamingoErrLogger.Println("Error opening Discord session: ", err)
		return
	}
	flamingoLogger.Println("Authenticated")
	discord.AddHandler(commandListener)

	// Wait here until CTRL-C or other term signal is received.
	flamingoLogger.Println("Flamingo is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	discord.Close()
}

func commandListener(session *discordgo.Session, m *discordgo.MessageCreate) {
	//Ignore bots
	if m.Author.Bot {
		return
	}

	if strings.HasPrefix(m.Message.Content, commandPrefix) {
		//This capacity is a magic number,
		//it's the average length of most command names
		var commandBuilder strings.Builder
		for _, v := range m.Message.Content {
			if v == '\u0020' {
				break
			}
			commandBuilder.WriteRune(v)
		}
		switch commandBuilder.String() {
		case commandPrefix + "strike":
			switch {
			case strings.HasPrefix(m.Message.Content, commandPrefix+"strike get"):
				if len(m.Mentions) > 0 {
					go strikeService.GetStrikesForUser(m.GuildID, m.ChannelID, m.Mentions[0].ID)
				} else {
					session.ChannelMessageSend(m.ChannelID, "Please mention a someone!")
				}
			case strings.HasPrefix(m.Message.Content, commandPrefix+"strike clear"):
				if len(m.Mentions) > 0 {
					go strikeService.ClearStrikesForUser(m.GuildID, m.ChannelID, m.Mentions[0].ID)
				} else {
					session.ChannelMessageSend(m.ChannelID, "Please mention a someone!")
				}
			default:
				if len(m.Mentions) > 0 {
					go strikeService.StrikeUser(m.GuildID, m.ChannelID, m.Mentions[0].ID)
				} else {
					session.ChannelMessageSend(m.ChannelID, "Please mention a someone!")
				}
			}
		case commandPrefix + "pasta":
			switch {
			case strings.HasPrefix(m.Message.Content, commandPrefix+"pasta get"):
				alias := strings.Replace(
					strings.SplitAfterN(
						m.Message.Content,
						commandPrefix+"pasta get",
						2)[1],
					" ", "", -1)
				if alias != "" {
					go pastaService.GetPasta(m.GuildID, m.ChannelID, alias)
				} else {
					session.ChannelMessageSend(m.ChannelID, "Please specify a copypasta!")
				}
			case strings.HasPrefix(m.Message.Content, commandPrefix+"pasta save"):
				aliasAndPasta := strings.SplitAfterN(
					m.Message.Content,
					commandPrefix+"pasta save",
					2)[1]
				aliasAndPasta = strings.TrimSpace(aliasAndPasta)
				var aliasBuilder strings.Builder
				for _, v := range aliasAndPasta {
					if v == '\u0020' {
						break
					}
					aliasBuilder.WriteRune(v)
				}
				alias := aliasBuilder.String()
				pasta := strings.SplitAfterN(
					aliasAndPasta,
					alias,
					2)[1]
				if alias == "" || pasta == "" {
					session.ChannelMessageSend(m.ChannelID, "Please specify an alias or copypasta!")
					return
				}
				go pastaService.SavePasta(m.GuildID, m.ChannelID, m.Author.ID, alias, pasta)
			}
		}
	}
}
