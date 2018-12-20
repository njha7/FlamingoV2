package flamingoservice

import (
	"bytes"
	"image"
	"image/png"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bwmarrin/discordgo"
	"github.com/nfnt/resize"
	"github.com/njha7/FlamingoV2/flamingolog"
)

const (
	reactServiceName = "React"
	reactBucket      = "flamingo-bot"
)

type ReactClient struct {
	DiscordSession     *discordgo.Session
	DynamoClient       *dynamodb.DynamoDB
	S3Client           *s3.S3
	ReactServiceLogger *log.Logger
	ReactErrorLogger   *log.Logger
}

func NewReactClient(discordSession *discordgo.Session, dynamoClient *dynamodb.DynamoDB, s3Client *s3.S3) *ReactClient {
	return &ReactClient{
		DiscordSession:     discordSession,
		DynamoClient:       dynamoClient,
		S3Client:           s3Client,
		ReactServiceLogger: flamingolog.BuildServiceLogger(strikeServiceName),
		ReactErrorLogger:   flamingolog.BuildServiceErrorLogger(strikeServiceName),
	}
}

func (reactClient *ReactClient) IsCommand(message string) bool {
	return false
}
func (reactClient *ReactClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	return
}

func (reactClient *ReactClient) PutReaction(channelID, userID, alias, url string) {
	response, err := http.Get(url)
	if err != nil {
		reactClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		reactClient.ReactErrorLogger.Println(err)
		return
	}
	defer response.Body.Close()

	image, _, err := image.Decode(response.Body)
	if err != nil {
		reactClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		reactClient.ReactErrorLogger.Println(err)
		return
	}

	x := float64(image.Bounds().Size().X)
	y := float64(image.Bounds().Size().Y)

	reszieRatio := 128.0 / x
	dx := uint(x * reszieRatio)
	dy := uint(y * reszieRatio)

	image = resize.Resize(dx, dy, image, resize.NearestNeighbor)

	buffer := new(bytes.Buffer)
	err = png.Encode(buffer, image)
	if err != nil {
		reactClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		reactClient.ReactErrorLogger.Println(err)
		return
	}
	_, err = reactClient.S3Client.PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(reactBucket),
		Key:           aws.String(buildReactionKey(userID, alias)),
		Body:          bytes.NewReader(buffer.Bytes()),
		ContentLength: aws.Int64(int64(len(buffer.Bytes()))),
		ContentType:   aws.String("image/png"),
		Tagging:       aws.String("app=flamingo&owner=" + userID),
		ACL:           aws.String("public-read"),
	})
	if err != nil {
		reactClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		reactClient.ReactErrorLogger.Println(err)
		return
	}
	reactClient.DiscordSession.ChannelMessageSend(channelID, alias+" reaction saved.")
}

func (reactClient *ReactClient) GetReaction(channelID, userID, alias string) {
	//Dirty way to test if object exists w/o querying it directly
	_, err := reactClient.S3Client.GetObjectAcl(&s3.GetObjectAclInput{
		Bucket: aws.String(reactBucket),
		Key:    aws.String(buildReactionKey(userID, alias)),
	})
	if err != nil {
		errMessage := "An error occured. Please try again later."
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchKey:
				errMessage = "No reaction with alias " + alias + " exists."
			}
		}
		reactClient.DiscordSession.ChannelMessageSend(channelID, errMessage)
		reactClient.ReactErrorLogger.Println(err)
		return
	}
	//Discord unmarshalling gives better results than sending the file
	reactClient.DiscordSession.ChannelMessageSend(channelID, "mfw "+buildReactionURL(userID, alias))
}

func (reactClient *ReactClient) DeleteReaction(channelID, userID, alias string) {
	key := buildReactionKey(userID, alias)
	_, err := reactClient.S3Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(reactBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		errMessage := "An error occured. Please try again later."
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchKey:
				errMessage = "No reaction with alias " + alias + " exists."
			}
		}
		reactClient.DiscordSession.ChannelMessageSend(channelID, errMessage)
		reactClient.ReactErrorLogger.Println(err)
		return
	}

	err = reactClient.S3Client.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(reactBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		reactClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		reactClient.ReactErrorLogger.Println(err)
		return
	}
	reactClient.DiscordSession.ChannelMessageSend(channelID, "Reaction with alias "+alias+" deleted.")
}

func buildReactionKey(userID, alias string) (key string) {
	key = userID + "/" + alias
	return
}

func buildReactionURL(userID, alias string) (s3url string) {
	s3url = "https://s3.amazonaws.com/" + reactBucket + "/" + userID + "/" + alias
	return
}
