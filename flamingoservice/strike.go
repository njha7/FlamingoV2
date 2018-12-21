package flamingoservice

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/njha7/FlamingoV2/assets"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
	"github.com/njha7/FlamingoV2/flamingolog"
)

const (
	strikeServiceName = "Strike"
)

// StrikeClient is responsible for handling "strike" commands
type StrikeClient struct {
	DynamoClient        *dynamodb.DynamoDB
	StrikeServiceLogger *log.Logger
	StrikeErrorLogger   *log.Logger
}

// Strike represents the schema for user strikes
type Strike struct {
	ID      string `dynamodbav:"guild!user"`
	Strikes int    `dynamodbav:"strikes"`
}

// StrikeKey is a convenience struct for marshaling Go types into a key for DDB requests for a given user
type StrikeKey struct {
	ID string `dynamodbav:"guild!user"`
}

// NewStrikeClient constructs a StrikeClient
func NewStrikeClient(dynamoClient *dynamodb.DynamoDB) *StrikeClient {
	return &StrikeClient{
		DynamoClient:        dynamoClient,
		StrikeServiceLogger: flamingolog.BuildServiceLogger(strikeServiceName),
		StrikeErrorLogger:   flamingolog.BuildServiceErrorLogger(strikeServiceName),
	}
}

// IsCommand identifies a message as a potential command
func (strikeClient *StrikeClient) IsCommand(message string) bool {
	return strings.HasPrefix(message, "strike")
}

// Handle parses a command message and performs the commanded action
func (strikeClient *StrikeClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	//first word is always "strike", safe to remove
	args := strings.SplitN(message.Content, " ", 3)[1:]
	fmt.Println(args)
	if len(args) < 1 {
		strikeClient.Help(session, message.ChannelID)
		return
	}
	//sub-commands of strike
	switch args[0] {
	case "get":
		switch len(message.Mentions) {
		case 0:
			session.ChannelMessageSend(message.ChannelID, "Please mention somone!")
		case 1:
			strikes, err := strikeClient.GetStrikesForUser(message.GuildID, message.ChannelID, message.Mentions[0].ID)
			ParseServiceResponse(session, message.ChannelID, strikes, err)
		default:
			strikes, err := strikeClient.BatchGetStrikesForUser(message.GuildID, message.ChannelID, message.Mentions)
			ParseServiceResponse(session, message.ChannelID, strikes, err)
		}

	case "clear":
		// Command disabled for now
		ParseServiceResponse(session, message.ChannelID, "strike clear has been disabled temporarily", nil)
		// if len(message.Mentions) < 1 {
		// 	session.ChannelMessageSend(message.ChannelID, "Please mention somone!")
		// 	return
		// }
		// for _, v := range message.Mentions {
		// 	strikes, err := strikeClient.ClearStrikesForUser(message.GuildID, message.ChannelID, v.ID)
		// 	ParseServiceResponse(session, message.ChannelID, strikes, err)
		// }
	case "help":
		strikeClient.Help(session, message.ChannelID)
	default:
		if len(message.Mentions) < 1 {
			session.ChannelMessageSend(message.ChannelID, "You must mention someone to strike!")
			strikeClient.Help(session, message.ChannelID)
			return
		}
		for _, v := range message.Mentions {
			strikes, err := strikeClient.StrikeUser(message.GuildID, message.ChannelID, v.ID)
			ParseServiceResponse(session, message.ChannelID, strikes, err)
		}
	}
}

// StrikeUser adds 1 to the strike count of a user
func (strikeClient *StrikeClient) StrikeUser(guildID, channelID, userID string) (string, error) {
	result, err := strikeClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:                 aws.String(assets.StrikeTableName),
		Key:                       buildStrikeKey(guildID, userID),
		UpdateExpression:          aws.String("ADD strikes :s"),
		ExpressionAttributeValues: buildStrikeUpdateExpression(1),
		ReturnValues:              aws.String("UPDATED_NEW"),
	})
	if err != nil {
		strikeClient.StrikeErrorLogger.Println(err)
		return "", nil
	}
	strikeCount, ok := result.Attributes["strikes"]
	if ok {
		return "<@" + userID + "> has " + *strikeCount.N + " strikes.", nil
	}
	strikeClient.StrikeErrorLogger.Printf("strike attribute not found after update guildID=%s userID=%s", guildID, userID)
	return "", errors.New("strike attribute not found after update")
}

// GetStrikesForUser retreives the number of strikes a user has
func (strikeClient *StrikeClient) GetStrikesForUser(guildID, channelID, userID string) (string, error) {
	result, err := strikeClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(assets.StrikeTableName),
		Key:       buildStrikeKey(guildID, userID),
	})
	if err != nil {
		strikeClient.StrikeErrorLogger.Println(err)
		return "", err
	}
	strikeCount, ok := result.Item["strikes"]
	if ok {
		if *strikeCount.N == "1" {
			return "<@" + userID + "> has " + *strikeCount.N + " strike.", nil
		}
		return "<@" + userID + "> has " + *strikeCount.N + " strikes.", nil
	}
	return "<@" + userID + "> has no strikes.", nil
}

// BatchGetStrikesForUser retreives the number of strikes for up to 20 users
func (strikeClient *StrikeClient) BatchGetStrikesForUser(guildID, channelID string, users []*discordgo.User) (interface{}, error) {
	if len(users) > 20 {
		return "You may only call get for up to 20 users. Please retry with fewer users.", nil
	}
	userIDxuserNameMap := make(map[string]string)
	keys := make([]map[string]*dynamodb.AttributeValue, 0, 20)
	for _, v := range users {
		userIDxuserNameMap[v.ID] = v.Username
		keys = append(keys, buildStrikeKey(guildID, v.ID))
	}
	result, err := strikeClient.DynamoClient.BatchGetItem(&dynamodb.BatchGetItemInput{
		RequestItems: map[string]*dynamodb.KeysAndAttributes{
			assets.StrikeTableName: &dynamodb.KeysAndAttributes{
				Keys: keys[:len(keys)],
			},
		},
	})
	if err != nil {
		strikeClient.StrikeErrorLogger.Println(err)
		return nil, err
	}
	// Error out on unproccessed keys. There should be no reason for this.
	if len(result.UnprocessedKeys) > 0 {
		strikeClient.StrikeErrorLogger.Printf("Unproccessed keys in BatchGetStrikesForUser request=%v result=%v unprocessed=%v\n",
			keys,
			result.Responses[assets.StrikeTableName],
			result.UnprocessedKeys[assets.StrikeTableName])
		return nil, errors.New("Unprocessed keys found in result")
	}
	strikes := make([]*discordgo.MessageEmbedField, 0, 20)
	for _, v := range result.Responses[assets.StrikeTableName] {
		strike := &Strike{}
		dynamodbattribute.UnmarshalMap(v, strike)
		//turn guild!user ddb key back into a userID from the map
		userID := strings.Split(strike.ID, "!")[1]
		username := userIDxuserNameMap[userID]
		strikeValue := strconv.Itoa(strike.Strikes)
		//remove keys that have values in the result
		delete(userIDxuserNameMap, userID)
		strikes = append(strikes, &discordgo.MessageEmbedField{

			Name:  username,
			Value: strikeValue,
		})
	}
	// remaining keys have no strikes, add these to the response
	for _, name := range userIDxuserNameMap {
		strikes = append(strikes, &discordgo.MessageEmbedField{
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
		Description: "3 strikes, you're out!",
		Fields:      strikes[:len(strikes)],
		Title:       "Strikes",
	}, nil
}

// ClearStrikesForUser resets the strikes of a user
func (strikeClient *StrikeClient) ClearStrikesForUser(guildID, channelID, userID string) (string, error) {
	_, err := strikeClient.DynamoClient.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(assets.StrikeTableName),
		Key:       buildStrikeKey(guildID, userID),
	})
	if err != nil {
		strikeClient.StrikeErrorLogger.Println(err)
		return "", nil
	}
	return "<@" + userID + "> has no strikes.", nil
}

// Help provides assistance with the strike command by sending a help dialogue
func (strikeClient *StrikeClient) Help(session *discordgo.Session, channelID string) {
	session.ChannelMessageSendEmbed(channelID,
		&discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{},
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: assets.AvatarURL,
			},
			Color:       0xff0000,
			Title:       "You need help!",
			Description: "The commands for strike are:",
			Fields: []*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name: "@user",
					Value: "Issues a strike to all mentioned users.\n" +
						"Usage: ```~strike @user1 @user2 ...```",
				},
				&discordgo.MessageEmbedField{
					Name: "get",
					Value: "Retrieves the strike count of mentioned users. \n" +
						"Usage: ```~strike @user1 @user2 ...```",
				},
				&discordgo.MessageEmbedField{
					Name:  "help",
					Value: "Shows this help message.",
				},
			},
		})
}

func buildStrikeKey(guildID, userID string) map[string]*dynamodb.AttributeValue {
	id := StrikeKey{
		ID: guildID + "!" + userID,
	}
	//err != nil will get caught in the request
	key, _ := dynamodbattribute.MarshalMap(id)
	return key
}

func buildStrikeUpdateExpression(update int) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue{
		":s": &dynamodb.AttributeValue{
			N: aws.String(strconv.Itoa(update)),
		},
	}
}
