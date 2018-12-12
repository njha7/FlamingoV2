package main

import (
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
)

const (
	pastaServiceName = "Pasta"
	pastaTableName   = "FlamingoPasta"
)

type PastaClient struct {
	DiscordSession     *discordgo.Session
	DynamoClient       *dynamodb.DynamoDB
	PastaServiceLogger *log.Logger
	PastaErrorLogger   *log.Logger
}

type Pasta struct {
	Guild string `dynamodbav:"guild"`
	Alias string `dynamodbav:"alias"`
	Owner string `dynamodbav:"owner"`
	Pasta string `dynamodbav:"pasta"`
}

type PastaKey struct {
	Guild string `dynamodbav:"guild"`
	Alias string `dynamodbav:"alias"`
}

func NewPastaClient(discordSession *discordgo.Session, dynamoClient *dynamodb.DynamoDB) *PastaClient {
	return &PastaClient{
		DiscordSession:     discordSession,
		DynamoClient:       dynamoClient,
		PastaServiceLogger: BuildServiceLogger(pastaServiceName),
		PastaErrorLogger:   BuildServiceErrorLogger(pastaServiceName),
	}
}

func (pastaClient *PastaClient) GetPasta(guildID, channelID, alias string) {
	result, err := pastaClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(pastaTableName),
		Key:       buildPastaKey(guildID, alias),
	})
	if err != nil {
		pastaClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}
	pasta, ok := result.Item["pasta"]
	if ok {
		pastaClient.DiscordSession.ChannelMessageSend(channelID, *pasta.S)
	} else {
		pastaClient.DiscordSession.ChannelMessageSend(channelID, "No copypasta with alias "+alias+" found.")
	}
}

func (pastaClient *PastaClient) SavePasta(guildID, channelID, owner, alias, pasta string) {
	_, err := pastaClient.DynamoClient.PutItem(&dynamodb.PutItemInput{
		TableName:           aws.String(pastaTableName),
		Item:                buildPasta(guildID, owner, alias, pasta),
		ConditionExpression: aws.String("attribute_not_exists(guild) and attribute_not_exists(alias)"),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				pastaClient.DiscordSession.ChannelMessageSend(channelID, "Copypasta with alias "+alias+" already exists.")
				return
			}
		}
		pastaClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}
	pastaClient.DiscordSession.ChannelMessageSend(channelID, "Copypasta with alias "+alias+" saved.")
}

func (pastaClient *PastaClient) ListPasta(guildID, channelID, userID string) {
	return
}

func buildPastaKey(guildID, alias string) map[string]*dynamodb.AttributeValue {
	id := PastaKey{
		Guild: guildID,
		Alias: alias,
	}
	//err != nil will get caught in the request
	key, _ := dynamodbattribute.MarshalMap(id)
	return key
}

func buildPasta(guildID, owner, alias, pasta string) map[string]*dynamodb.AttributeValue {
	pastaItem := Pasta{
		Guild: guildID,
		Owner: owner,
		Alias: alias,
		Pasta: pasta,
	}
	//err != nil will get caught in the request
	item, _ := dynamodbattribute.MarshalMap(pastaItem)
	return item
}
