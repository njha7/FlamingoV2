package main

import (
	"bytes"
	"image"
	"image/png"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bwmarrin/discordgo"
	"github.com/nfnt/resize"
)

const (
	reactServiceName = "React"
	reactBucket      = "flamingo-bot"
)

type ReactClient struct {
	DiscordSession     *discordgo.Session
	S3Client           *s3.S3
	ReactServiceLogger *log.Logger
	ReactErrorLogger   *log.Logger
}

func NewReactClient(discordSession *discordgo.Session, s3Client *s3.S3) *ReactClient {
	return &ReactClient{
		DiscordSession:     discordSession,
		S3Client:           s3Client,
		ReactServiceLogger: BuildServiceLogger(strikeServiceName),
		ReactErrorLogger:   BuildServiceErrorLogger(strikeServiceName),
	}
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

	ratio := x / y
	reszieRatio := 128.0 / x
	dx := uint(x * reszieRatio)
	dy := uint(y * reszieRatio * ratio)

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

func (reactClient *ReactClient) ListReactions(channelID, userID string) {
	dmChannel, err := reactClient.DiscordSession.UserChannelCreate(userID)
	if err != nil {
		reactClient.DiscordSession.ChannelMessageSend(channelID, "An error occurred. Could not DM <@"+userID+">.")
		reactClient.ReactErrorLogger.Println(err)
		return
	}
	err = reactClient.S3Client.ListObjectsV2Pages(
		&s3.ListObjectsV2Input{
			Bucket:  aws.String(reactBucket),
			Prefix:  aws.String(buildReactionKey(userID, "")),
			MaxKeys: aws.Int64(30),
		},
		func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			reactionList := make([]*discordgo.MessageEmbedField, 0, 30)
			if *page.KeyCount < 1 {
				reactionList = append(reactionList, &discordgo.MessageEmbedField{
					Name:   "No reactions found.",
					Value:  "):",
					Inline: true,
				})
			}
			for _, v := range page.Contents {
				alias := strings.Split(*v.Key, "/")[1]
				reactionList = append(reactionList, &discordgo.MessageEmbedField{
					Name: alias,
					Value:  "https://s3.amazonaws.com/"+reactBucket+"/"+*v.Key,
					Inline: true,
				})
			}
			reactClient.DiscordSession.ChannelMessageSendEmbed(dmChannel.ID,
				&discordgo.MessageEmbed{
					Author: &discordgo.MessageEmbedAuthor{},
					Thumbnail: &discordgo.MessageEmbedThumbnail{
						URL: "https://cdn.discordapp.com/avatars/518879406509391878/ca293c592d560f09d958e85166938e88.png?size=256",
					},
					Color:       0x0000ff,
					Description: "A list of your reactions",
					Fields:      reactionList[:len(reactionList)],
					Title:       "Your reactions",
				})
			if lastPage {
				return false
			}
			return true
		})
}

func buildReactionKey(userID, alias string) (key string) {
	key = userID + "/" + alias
	return
}

func buildReactionURL(userID, alias string) (s3url string) {
	s3url = "https://s3.amazonaws.com/" + reactBucket + "/" + userID + "/" + alias
	return
}
