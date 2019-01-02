package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bwmarrin/discordgo"
	"github.com/njha7/FlamingoV2/flamingolog"
	"github.com/njha7/FlamingoV2/flamingoservice"
)

var (
	DISCORD_TOKEN, AWS_ACCESS_KEY, AWS_SECRET_KEY, REGION string
	local                                                 bool
	flamingoLogger                                        *log.Logger
	flamingoErrLogger                                     *log.Logger
	commandServices                                       []flamingoservice.FlamingoService
)

func init() {
	flamingoLogger = flamingolog.BuildServiceLogger("Flamingo")
	flamingoErrLogger = flamingolog.BuildServiceErrorLogger("Flamingo")
	//Dumb and lazy hack
	flag.BoolVar(&local, "local", false, "Flag for running waimote in local test mode.")
	flag.StringVar(&DISCORD_TOKEN, "t", "", "Discord bot token.")
	flag.StringVar(&AWS_ACCESS_KEY, "ak", "", "AWS Access Key")
	flag.StringVar(&AWS_SECRET_KEY, "sk", "", "AWS Secret Key")
	flag.StringVar(&REGION, "r", "", "AWS Region")
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
		REGION = os.Getenv("REGION")
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
	ddb := dynamodb.New(awsSess, aws.NewConfig().WithRegion(REGION))
	// Create S3 service client with a specific Region.
	s3 := s3.New(awsSess, aws.NewConfig().WithRegion(REGION))
	cw := cloudwatch.New(awsSess, aws.NewConfig().WithRegion(REGION))
	//Flamingo service Client construction
	metricsClient := &flamingolog.FlamingoMetricsClient{
		CloudWatchAgent: cw,
		Local:           local,
	}
	authClient := flamingoservice.NewAuthClient(discord, ddb, metricsClient)

	commandServices = []flamingoservice.FlamingoService{
		flamingoservice.NewStrikeClient(ddb, metricsClient),
		flamingoservice.NewPastaClient(ddb, metricsClient),
		flamingoservice.NewReactClient(s3, metricsClient, authClient),
		authClient,
	}
	//Start Flamingo
	err = discord.Open()
	if err != nil {
		flamingoErrLogger.Println("Error opening Discord session: ", err)
		return
	}
	flamingoLogger.Println("Authenticated")
	discord.AddHandler(commandListener)
	discord.AddHandler(authSetup(authClient))

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

	if strings.HasPrefix(m.Message.Content, flamingoservice.CommandPrefix) {
		for _, v := range commandServices {
			//Command services are unaware of the prefix
			if v.IsCommand(m.Content[len(flamingoservice.CommandPrefix):]) {
				go v.Handle(session, m.Message)
				return
			}
		}
	}
}

func authSetup(authClient *flamingoservice.AuthClient) func(*discordgo.Session, *discordgo.GuildCreate) {
	return func(session *discordgo.Session, gc *discordgo.GuildCreate) {
		timeStamp, err := gc.JoinedAt.Parse()
		if err != nil {
			flamingoErrLogger.Println(err)
			return
		}
		//Join time <30s is an indicator of joining recently as opposed to reconnecting
		if timeStamp.Unix() > time.Now().Unix()-30 {
			flamingoLogger.Printf("Joined %s. Setting permissive flag.\n", gc.Guild.ID)
			err := authClient.SetDefaultPermissiveFlagValue(gc.Guild.ID)
			if err != nil {
				flamingoErrLogger.Printf("An error occured while setting permissive flag for %s", gc.Guild.ID)
				flamingoErrLogger.Println(err)
			}
			authClient.SetPermission(gc.Guild.ID, gc.OwnerID, "auth", "", false, true)
		}
	}
}
