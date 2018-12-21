package flamingoservice

import (
	"bytes"
	"image"
	"image/png"
	"log"
	"net/http"
	"strings"

	"github.com/njha7/FlamingoV2/assets"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bwmarrin/discordgo"
	"github.com/nfnt/resize"
	"github.com/njha7/FlamingoV2/flamingolog"
)

const (
	reactServiceName = "React"
)

// ReactClient is responsible for handling "react" commands
type ReactClient struct {
	S3Client           *s3.S3
	ReactServiceLogger *log.Logger
	ReactErrorLogger   *log.Logger
}

// NewReactClient constructs a ReactClient
func NewReactClient(s3Client *s3.S3) *ReactClient {
	return &ReactClient{
		S3Client:           s3Client,
		ReactServiceLogger: flamingolog.BuildServiceLogger(strikeServiceName),
		ReactErrorLogger:   flamingolog.BuildServiceErrorLogger(strikeServiceName),
	}
}

// IsCommand identifies a message as a potential command
func (reactClient *ReactClient) IsCommand(message string) bool {
	return strings.HasPrefix(message, "react")
}

// Handle parses a command message and performs the commanded action
func (reactClient *ReactClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	//first word is always "react", safe to remove
	args := strings.Fields(message.Content)[1:]
	if len(args) < 1 {
		reactClient.Help(session, message.ChannelID)
		return
	}
	//sub-commands of react
	switch args[0] {
	case "get":
		if len(args) < 2 {
			session.ChannelMessageSend(message.ChannelID, "Please specify an alias.")
			return
		}
		reaction, err := reactClient.GetReaction(message.ChannelID, message.Author.ID, args[1])
		ParseServiceResponse(session, message.ChannelID, reaction, err)
	case "save":
		if len(args) < 2 || len(message.Attachments) < 1 {
			session.ChannelMessageSend(message.ChannelID, "Please upload an image or specify an alias.")
			return
		}
		_, err := reactClient.PutReaction(message.ChannelID, message.Author.ID, args[1], message.Attachments[0].URL)
		ParseServiceResponse(session, message.ChannelID, "Reaction with alias "+args[1]+" saved.", err)
	case "delete":
		if len(args) < 2 {
			session.ChannelMessageSend(message.ChannelID, "Please specify an alias.")
			return
		}
		result, err := reactClient.DeleteReaction(message.ChannelID, message.Author.ID, args[1])
		ParseServiceResponse(session, message.ChannelID, result, err)
	case "list":
		reactClient.ListReactions(session, message.ChannelID, message.Author.ID)
	case "help":
		reactClient.Help(session, message.ChannelID)
	default:
		reactClient.Help(session, message.ChannelID)
	}
}

// PutReaction saves an aspect-ratio preserved thumbnail of an image for later use
func (reactClient *ReactClient) PutReaction(channelID, userID, alias, url string) (bool, error) {
	response, err := http.Get(url)
	if err != nil {
		reactClient.ReactErrorLogger.Println(err)
		return false, err
	}
	defer response.Body.Close()

	image, _, err := image.Decode(response.Body)
	if err != nil {
		reactClient.ReactErrorLogger.Println(err)
		return false, err
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
		reactClient.ReactErrorLogger.Println(err)
		return false, err
	}
	_, err = reactClient.S3Client.PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(assets.BucketName),
		Key:           aws.String(buildReactionKey(userID, alias)),
		Body:          bytes.NewReader(buffer.Bytes()),
		ContentLength: aws.Int64(int64(len(buffer.Bytes()))),
		ContentType:   aws.String("image/png"),
		Tagging:       aws.String("app=flamingo&owner=" + userID),
		ACL:           aws.String("public-read"),
	})
	if err != nil {
		reactClient.ReactErrorLogger.Println(err)
		return false, err
	}
	return true, nil
	// reactClient.DiscordSession.ChannelMessageSend(channelID, alias+" reaction saved.")
}

// GetReaction retrieves a reaction by alias and returns the url
func (reactClient *ReactClient) GetReaction(channelID, userID, alias string) (string, error) {
	//Dirty way to test if object exists w/o querying it directly (ACL is smaller than the image)
	_, err := reactClient.S3Client.GetObjectAcl(&s3.GetObjectAclInput{
		Bucket: aws.String(assets.BucketName),
		Key:    aws.String(buildReactionKey(userID, alias)),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchKey:
				return "No reaction with alias " + alias + " exists.", nil
			default:
				reactClient.ReactErrorLogger.Println(err)
				return "", err
			}
		} else {
			reactClient.ReactErrorLogger.Println(err)
			return "", err
		}
	}
	//Discord unmarshalling gives better results than sending the file
	return "mfw " + buildReactionURL(userID, alias), nil
}

// DeleteReaction deletes a users reaction image by alias
func (reactClient *ReactClient) DeleteReaction(channelID, userID, alias string) (string, error) {
	key := buildReactionKey(userID, alias)
	_, err := reactClient.S3Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(assets.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchKey:
				return "No reaction with alias " + alias + " exists.", nil
			default:
				reactClient.ReactErrorLogger.Println(err)
				return "", err
			}
		} else {
			reactClient.ReactErrorLogger.Println(err)
			return "", err
		}
	}

	err = reactClient.S3Client.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(assets.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		reactClient.ReactErrorLogger.Println(err)
		return "", err
	}
	return "Reaction with alias " + alias + " deleted.", nil
}

// ListReactions lists all reactions a user has saved via dm
func (reactClient *ReactClient) ListReactions(session *discordgo.Session, channelID, userID string) {
	dmChannel, err := session.UserChannelCreate(userID)
	if err != nil {
		session.ChannelMessageSend(channelID, "An error occurred. Could not DM <@"+userID+">.")
		reactClient.ReactErrorLogger.Println(err)
		return
	}
	err = reactClient.S3Client.ListObjectsV2Pages(
		&s3.ListObjectsV2Input{
			Bucket:  aws.String(assets.BucketName),
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
					Name:   alias,
					Value:  "https://s3.amazonaws.com/" + assets.BucketName + "/" + *v.Key,
					Inline: true,
				})
			}
			session.ChannelMessageSendEmbed(dmChannel.ID,
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

// Help provides assistance with the react command by sending a help dialogue
func (reactClient *ReactClient) Help(session *discordgo.Session, channelID string) {
	session.ChannelMessageSendEmbed(channelID,
		&discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{},
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: assets.AvatarURL,
			},
			Color:       0xff0000,
			Title:       "You need help!",
			Description: "The commands for react are:",
			Fields: []*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name: "get",
					Value: "Retrieves a reaction image by alias and posts it. Alias can by any alphanumeric string with no whitespace.\n" +
						"Usage: ```~react get $alias```",
				},
				&discordgo.MessageEmbedField{
					Name: "save",
					Value: "Saves a new a reaction by alias. Reactions are images uploaded to Discord. They are thumbnailed and saved for later reacall. Alias can by any alphanumeric string with no whitespace. Can be used to overwrite an existing reaction.\n" +
						"Usage: ```~react save $alias```",
				},
				&discordgo.MessageEmbedField{
					Name: "delete",
					Value: "Deletes a reaction image and makes it unavailable for use. Alias can by any alphanumeric string with no whitespace.\n" +
						"Usage: ```~react delete $alias```",
				},
				&discordgo.MessageEmbedField{
					Name: "list",
					Value: "Retrieves a list of all the reaction images saved and DMs them to the caller.\n" +
						"Usage: ```~react list```",
				},
				&discordgo.MessageEmbedField{
					Name:  "help",
					Value: "Shows this help message.",
				},
			},
		})
}

func buildReactionKey(userID, alias string) (key string) {
	key = userID + "/" + alias
	return
}

func buildReactionURL(userID, alias string) (s3url string) {
	s3url = "https://s3.amazonaws.com/" + assets.BucketName + "/" + userID + "/" + alias
	return
}
