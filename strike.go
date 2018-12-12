package main

import (
	"log"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
)

const (
	strikeServiceName = "Strike"
	strikeTableName   = "FlamingoStrikes"
)

type StrikeClient struct {
	DiscordSession      *discordgo.Session
	DynamoClient        *dynamodb.DynamoDB
	StrikeServiceLogger *log.Logger
	StrikeErrorLogger   *log.Logger
}

type Strike struct {
	ID      string `dynamodbav:"guild!user"`
	Strikes int    `dynamodbav:"strikes"`
}

type StrikeKey struct {
	ID string `dynamodbav:"guild!user"`
}

func NewStrikeClient(discordSession *discordgo.Session, dynamoClient *dynamodb.DynamoDB) *StrikeClient {
	return &StrikeClient{
		DiscordSession:      discordSession,
		DynamoClient:        dynamoClient,
		StrikeServiceLogger: BuildServiceLogger(strikeServiceName),
		StrikeErrorLogger:   BuildServiceErrorLogger(strikeServiceName),
	}
}

func (strikeClient *StrikeClient) StrikeUser(guildID, channelID, userID string) {
	if userID == "132651817409445888" {
		// tdorn is immune
		strikeClient.DiscordSession.ChannelMessageSend(channelID, "Nice try bucko, tdorn is immune.")
		return;
	}
	result, err := strikeClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:                 aws.String(strikeTableName),
		Key:                       buildKey(guildID, userID),
		UpdateExpression:          aws.String("ADD strikes :s"),
		ExpressionAttributeValues: buildUpdateExpression(1),
		ReturnValues:              aws.String("UPDATED_NEW"),
	})
	if err != nil {
		strikeClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		strikeClient.StrikeErrorLogger.Println(err)
		return
	}
	strikeCount, ok := result.Attributes["strikes"]
	if ok {
		strikeClient.DiscordSession.ChannelMessageSend(channelID,
			"<@"+userID+"> has "+*strikeCount.N+" strikes.")
	} else {
		strikeClient.StrikeErrorLogger.Printf("strikes not found in %v\n", strikeCount)
	}
}

func (strikeClient *StrikeClient) GetStrikesForUser(guildID, channelID, userID string) {
	result, err := strikeClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(strikeTableName),
		Key:       buildKey(guildID, userID),
	})
	if err != nil {
		strikeClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		strikeClient.StrikeErrorLogger.Println(err)
		return
	}
	strikeCount, ok := result.Item["strikes"]
	if ok {
		strikeClient.DiscordSession.ChannelMessageSend(channelID,
			"<@"+userID+"> has "+*strikeCount.N+" strikes.")
	} else {
		strikeClient.DiscordSession.ChannelMessageSend(channelID,
			"<@"+userID+"> has no strikes.")
	}
}

func (strikeClient *StrikeClient) ClearStrikesForUser(guildID, channelID, userID string) {
	_, err := strikeClient.DynamoClient.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(strikeTableName),
		Key:       buildKey(guildID, userID),
	})
	if err != nil {
		strikeClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		strikeClient.StrikeErrorLogger.Println(err)
		return
	}
	strikeClient.DiscordSession.ChannelMessageSend(channelID,
		"<@"+userID+"> has no strikes.")
}

func buildKey(guildID, userID string) map[string]*dynamodb.AttributeValue {
	id := StrikeKey{
		ID: guildID + "!" + userID,
	}
	//err != nil will get caught in the request
	key, _ := dynamodbattribute.MarshalMap(id)
	return key
}

func buildUpdateExpression(update int) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue{
		":s": &dynamodb.AttributeValue{
			N: aws.String(strconv.Itoa(update)),
		},
	}
}
