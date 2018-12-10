package main

import (
	"flag"
	"strings"
	"os"
	"log"
	"os/signal"
	"syscall"
	"github.com/bwmarrin/discordgo"
)


const (
	commandPrefix string = "!" 
	bucket string = "flamingo-bot"
)

var (
	DISCORD_TOKEN, AWS_ACCESS_KEY, AWS_SECRET_KEY string
	local bool
	flamingoLogger *log.Logger
	flamingoErrLogger *log.Logger
)

func init()  {
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

func commandListener(s *discordgo.Session, m *discordgo.MessageCreate) {
	//Ignore bots
	if m.Author.Bot {
		return
	}

	if strings.HasPrefix(m.Message.Content, commandPrefix) {
		//This capacity is a magic number,
		//it's the average length of most command names
		commandBuilder := &strings.Builder{}
		for _, v := range m.Message.Content {
			if v == '\u0020' {
				break
			}
			commandBuilder.WriteRune(v)
		}
	}
}