package flamingoservice

import (
	"FlamingoV2/assets"
	"FlamingoV2/flamingolog"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
)

const (
	strokeServiceName = "Stroke"
	strokeCommand     = "stroke"
)

// StrokeClient is responsible for handling "stroke" commands
type StrokeClient struct {
	DynamoClient        *dynamodb.DynamoDB
	MetricsClient       *flamingolog.FlamingoMetricsClient
	AuthClient          *AuthClient
	StrokeServiceLogger *log.Logger
	StrokeErrorLogger   *log.Logger
}

// Stroke represents the schema for user strokes
type Stroke struct {
	ID      string `dynamodbav:"guild!user"`
	Strokes int    `dynamodbav:"strokes"`
	Strikes int    `dynamodbav:"strikes"`
}

// StrokeKey is a convenience struct for marshalling Go types into a key for DDB requests for a given user
type StrokeKey struct {
	ID string `dynamodbav:"guild!user"`
}

// NewStrokeClient constructs a StrokeClient
func NewStrokeClient(dynamoClient *dynamodb.DynamoDB, metricsClient *flamingolog.FlamingoMetricsClient, authClient *AuthClient) *StrokeClient {
	return &StrokeClient{
		DynamoClient:        dynamoClient,
		MetricsClient:       metricsClient,
		AuthClient:          authClient,
		StrokeServiceLogger: flamingolog.BuildServiceLogger(strokeServiceName),
		StrokeErrorLogger:   flamingolog.BuildServiceErrorLogger(strokeServiceName),
	}
}

// IsCommand identifies a message as a potential command
func (strokeClient *StrokeClient) IsCommand(message string) bool {
	return strings.HasPrefix(message, strokeCommand)
}

// Handle parses a command message and performs the commanded action
func (strokeClient *StrokeClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	//first word is always "stroke", safe to remove
	args := strings.SplitN(message.Content, " ", 3)[1:]
	if len(args) < 1 {
		strokeClient.Help(session, message.ChannelID)
		return
	}
	//sub-commands of stroke
	switch args[0] {
	case "get":
		switch len(message.Mentions) {
		case 0:
			session.ChannelMessageSend(message.ChannelID, "Please mention somone!")
		case 1:
			strokes, err := strokeClient.GetStrokesForUser(message.GuildID, message.ChannelID, message.Mentions[0].ID)
			ParseServiceResponse(session, message.ChannelID, strokes, err)
		default:
			strokes, err := strokeClient.BatchGetStrokesForUser(message.GuildID, message.ChannelID, message.Mentions)
			ParseServiceResponse(session, message.ChannelID, strokes, err)
		}

	case "clear":
		if strokeClient.AuthClient.Authorize(message.GuildID, message.Author.ID, strokeCommand, "clear") {
			if len(message.Mentions) < 1 {
				session.ChannelMessageSend(message.ChannelID, "Please mention somone!")
				return
			}
			for _, v := range message.Mentions {
				strokes, err := strokeClient.ClearStrokesForUser(message.GuildID, message.ChannelID, v.ID)
				ParseServiceResponse(session, message.ChannelID, strokes, err)
			}
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "super":
		if len(message.Mentions) < 1 {
			session.ChannelMessageSend(message.ChannelID, "You must mention someone to stroke!")
			strokeClient.Help(session, message.ChannelID)
			return
		}
		if strokeClient.AuthClient.Authorize(message.GuildID, message.Author.ID, strokeCommand, "super") {
			for _, v := range message.Mentions {
				strokes, err := strokeClient.SuperStrokeUser(message.GuildID, message.ChannelID, v.ID)
				ParseServiceResponse(session, message.ChannelID, strokes, err)
			}
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "status":
		if len(message.Mentions) < 1 {
			session.ChannelMessageSend(message.ChannelID, "You must mention someone to get their status!")
			strokeClient.Help(session, message.ChannelID)
			return
		}
		if strokeClient.AuthClient.Authorize(message.GuildID, message.Author.ID, strokeCommand, "status") {
			for _, v := range message.Mentions {
				strokes, err := strokeClient.GetStrokeStatusForUser(message.GuildID, message.ChannelID, v.ID)
				ParseServiceResponse(session, message.ChannelID, strokes, err)
			}
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "help":
		strokeClient.Help(session, message.ChannelID)
	default:
		if len(message.Mentions) < 1 {
			session.ChannelMessageSend(message.ChannelID, "You must mention someone to stroke!")
			strokeClient.Help(session, message.ChannelID)
			return
		}
		if strokeClient.AuthClient.Authorize(message.GuildID, message.Author.ID, strokeCommand, "") {
			for _, v := range message.Mentions {
				strokes, err := strokeClient.StrokeUser(message.GuildID, message.ChannelID, v.ID)
				ParseServiceResponse(session, message.ChannelID, strokes, err)
			}
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	}
}

// StrokeUser adds 1 to the stroke count of a user
func (strokeClient *StrokeClient) StrokeUser(guildID, channelID, userID string) (string, error) {
	result, err := strokeClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:                 aws.String(assets.StrikeTableName),
		Key:                       buildStrokeKey(guildID, userID),
		UpdateExpression:          aws.String("ADD strokes :s"),
		ExpressionAttributeValues: buildStrokeUpdateExpression(1),
		ReturnValues:              aws.String("UPDATED_NEW"),
	})
	if err != nil {
		strokeClient.StrokeErrorLogger.Println(err)
		return "", nil
	}
	strokeCount, ok := result.Attributes["strokes"]
	if ok {
		return "<@" + userID + "> has " + *strokeCount.N + " strokes.", nil
	}
	strokeClient.StrokeErrorLogger.Printf("stroke attribute not found after update guildID=%s userID=%s", guildID, userID)
	return "", errors.New("stroke attribute not found after update")
}

// SuperStrokeUser adds 10 to the stroke count of a user
func (strokeClient *StrokeClient) SuperStrokeUser(guildID, channelID, userID string) (string, error) {
	result, err := strokeClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:                 aws.String(assets.StrikeTableName),
		Key:                       buildStrokeKey(guildID, userID),
		UpdateExpression:          aws.String("ADD strokes :s"),
		ExpressionAttributeValues: buildStrokeUpdateExpression(10),
		ReturnValues:              aws.String("UPDATED_NEW"),
	})
	if err != nil {
		strokeClient.StrokeErrorLogger.Println(err)
		return "", nil
	}
	strokeCount, ok := result.Attributes["strokes"]
	if ok {
		return "<@" + userID + "> has " + *strokeCount.N + " strokes.", nil
	}
	strokeClient.StrokeErrorLogger.Printf("stroke attribute not found after update guildID=%s userID=%s", guildID, userID)
	return "", errors.New("stroke attribute not found after update")
}

// GetStrokesForUser retreives the number of strokes a user has
func (strokeClient *StrokeClient) GetStrokesForUser(guildID, channelID, userID string) (string, error) {
	result, err := strokeClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(assets.StrikeTableName),
		Key:       buildStrokeKey(guildID, userID),
	})
	if err != nil {
		strokeClient.StrokeErrorLogger.Println(err)
		return "", err
	}
	strokeCount, ok := result.Item["strokes"]
	if ok {
		if *strokeCount.N == "1" {
			return "<@" + userID + "> has " + *strokeCount.N + " stroke.", nil
		}
		return "<@" + userID + "> has " + *strokeCount.N + " strokes.", nil
	}
	return "<@" + userID + "> has no strokes.", nil
}

// GetStrokeStatusForUser retreives the user's status (Chad or Virgin).
func (strokeClient *StrokeClient) GetStrokeStatusForUser(guildID, channelID, userID string) (string, error) {
	strokesResult, strokesErr := strokeClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(assets.StrikeTableName),
		Key:       buildStrokeKey(guildID, userID),
	})
	if strokesErr != nil {
		strokeClient.StrokeErrorLogger.Println(strokesErr)
		return "", strokesErr
	}
	stroke := Stroke{}
	err := dynamodbattribute.UnmarshalMap(strokesResult.Item, &stroke)
	if err != nil {
		strokeClient.StrokeErrorLogger.Println(err)
		return "", err
	}
	strokesCount := stroke.Strokes
	strikesCount := stroke.Strikes
	if strokesCount > strikesCount {
		return fmt.Sprintf("<@%s> Status: Chad.\n User has more strokes (%d) than strikes (%d).", userID, strokesCount, strikesCount), nil
	} else {
		if strokesCount == strikesCount {
			return fmt.Sprintf("<@%s> Status: Virgin.\n User needs one more stroke to become a Chad!",
				userID), nil
		} else {
			return fmt.Sprintf("<@%s> Status: Virgin.\n User needs %d more strokes to become a Chad!",
				userID, strikesCount-strokesCount+1), nil
		}
	}
}

// BatchGetStrokesForUser retreives the number of strokes for up to 20 users
func (strokeClient *StrokeClient) BatchGetStrokesForUser(guildID, channelID string, users []*discordgo.User) (interface{}, error) {
	if len(users) > 20 {
		return "You may only call get for up to 20 users. Please retry with fewer users.", nil
	}
	userIDxuserNameMap := make(map[string]string)
	keys := make([]map[string]*dynamodb.AttributeValue, 0, 20)
	for _, v := range users {
		userIDxuserNameMap[v.ID] = v.Username
		keys = append(keys, buildStrokeKey(guildID, v.ID))
	}
	result, err := strokeClient.DynamoClient.BatchGetItem(&dynamodb.BatchGetItemInput{
		RequestItems: map[string]*dynamodb.KeysAndAttributes{
			assets.StrikeTableName: &dynamodb.KeysAndAttributes{
				Keys: keys[:len(keys)],
			},
		},
	})
	if err != nil {
		strokeClient.StrokeErrorLogger.Println(err)
		return nil, err
	}
	// Error out on unproccessed keys. There should be no reason for this.
	if len(result.UnprocessedKeys) > 0 {
		strokeClient.StrokeErrorLogger.Printf("Unproccessed keys in BatchGetStrokesForUser request=%v result=%v unprocessed=%v\n",
			keys,
			result.Responses[assets.StrikeTableName],
			result.UnprocessedKeys[assets.StrikeTableName])
		return nil, errors.New("Unprocessed keys found in result")
	}
	strokes := make([]*discordgo.MessageEmbedField, 0, 20)
	for _, v := range result.Responses[assets.StrikeTableName] {
		stroke := &Stroke{}
		dynamodbattribute.UnmarshalMap(v, stroke)
		//turn guild!user ddb key back into a userID from the map
		userID := strings.Split(stroke.ID, "!")[1]
		username := userIDxuserNameMap[userID]
		strokeValue := strconv.Itoa(stroke.Strokes)
		//remove keys that have values in the result
		delete(userIDxuserNameMap, userID)
		strokes = append(strokes, &discordgo.MessageEmbedField{

			Name:  username,
			Value: strokeValue,
		})
	}
	// remaining keys have no strokes, add these to the response
	for _, name := range userIDxuserNameMap {
		strokes = append(strokes, &discordgo.MessageEmbedField{
			Name:  name,
			Value: "0",
		})
	}

	return &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: assets.AvatarURL,
		},
		Color:       0xd6c22f,
		Description: "3 strokes, you're out!",
		Fields:      strokes[:len(strokes)],
		Title:       "Strokes",
	}, nil
}

// ClearStrokesForUser resets the strokes of a user
func (strokeClient *StrokeClient) ClearStrokesForUser(guildID, channelID, userID string) (string, error) {
	_, err := strokeClient.DynamoClient.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(assets.StrikeTableName),
		Key:       buildStrokeKey(guildID, userID),
	})
	if err != nil {
		strokeClient.StrokeErrorLogger.Println(err)
		return "", nil
	}
	return "<@" + userID + "> has no strokes.", nil
}

// Help provides assistance with the stroke command by sending a help dialogue
func (strokeClient *StrokeClient) Help(session *discordgo.Session, channelID string) {
	session.ChannelMessageSendEmbed(channelID,
		&discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{},
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: assets.AvatarURL,
			},
			Color:       0xff0000,
			Title:       "You need help!",
			Description: "The commands for stroke are:",
			Fields: []*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name: "@user",
					Value: "Issues a stroke to all mentioned users.\n" +
						"Usage: ```~stroke @user1 @user2 ...```",
				},
				&discordgo.MessageEmbedField{
					Name: "get",
					Value: "Retrieves the stroke count of mentioned users. \n" +
						"Usage: ```~stroke get @user1 @user2 ...```",
				},
				&discordgo.MessageEmbedField{
					Name: "status",
					Value: "Retrieves the Chad/Virgin status of mentioned users.\n" +
						"Having more strokes than strikes makes you a Chad.\n" +
						"Usage: ```~stroke status @user1 @user2 ...```",
				},
				&discordgo.MessageEmbedField{
					Name:  "help",
					Value: "Shows this help message.",
				},
			},
		})
}

func buildStrokeKey(guildID, userID string) map[string]*dynamodb.AttributeValue {
	id := StrokeKey{
		ID: guildID + "!" + userID,
	}
	//err != nil will get caught in the request
	key, _ := dynamodbattribute.MarshalMap(id)
	return key
}

func buildStrokeUpdateExpression(update int) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue{
		":s": &dynamodb.AttributeValue{
			N: aws.String(strconv.Itoa(update)),
		},
	}
}
