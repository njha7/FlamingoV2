package main

import (
	"log"
	"strconv"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
)

const (
	serviceName = "Strike"
	tableName = "FlamingoStrikes"
)

type StrikeClient struct {
	DiscordSession *discordgo.Session
	DynamoClient *dynamodb.DynamoDB
	StrikeServiceLogger *log.Logger
	StrikeErrorLogger *log.Logger 
}

type Strike struct {
	ID string `dynamodbav:guild!user`
	Strikes int `dynamodbav:strikes`
}

func (strikeClient *StrikeClient) New(discordSession *discordgo.Session, dynamoClient *dynamodb.DynamoDB) *StrikeClient {
	return &StrikeClient{
		DiscordSession: discordSession,
		DynamoClient: dynamoClient,
		StrikeServiceLogger: BuildServiceLogger(serviceName),
		StrikeErrorLogger: BuildServiceErrorLogger(serviceName),
	}
}

func (strikeClient *StrikeClient) StrikeUser(guildId, userId string) error {
	_, err := strikeClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: *tableName,
		Key: buildKey(guildId, userId),
		UpdateExpression: "ADD strikes :s",
		ExpressionAttributeValues: buildUpdateExpression(1),
		ReturnValues: "UPDATED_NEW",
	})
	return err
}

func buildKey(guildId, userId string) map[string]*dynamodb.AttributeValue {
	id := &Strike{
		ID: guildId + "!" + userId,
	}
	//err != nil will get caught in the request
	key, err := dynamodbattribute.MarshalMap(id)
	return key
}

func buildUpdateExpression(update int) map[string]*dynamodb.AttributeValue {
	var av dynamodb.Number = strconv.Itoa(update)
	return map[string]*dynamodb.AttributeValue{
		":s": &av,
	}
}