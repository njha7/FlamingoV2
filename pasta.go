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

type PastaPage struct {
	Guild string `dynamodbav:"guild"`
	Alias string `dynamodbav:"alias"`
	Key   string `dynamodbav:"marker"`
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

func (pastaClient *PastaClient) ListPasta(guildID, channelID string) {
	pastaList := &dynamodb.QueryInput{
		TableName:              aws.String(pastaTableName),
		KeyConditionExpression: aws.String("guild=:g"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":g": &dynamodb.AttributeValue{
				S: aws.String(guildID),
			},
		},
		Limit: aws.Int64(15),
	}

	result, err := pastaClient.DynamoClient.Query(pastaList)
	if err != nil {
		pastaClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}
	//List pastas in chat
	guildPastaList := buildPastaPage(result)
	sendPastaPage(pastaClient.DiscordSession, channelID, guildPastaList)
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


func buildPastaPage(pastas *dynamodb.QueryOutput) []*discordgo.MessageEmbedField {
	//List pastas in chat
	guildPastaList := make([]*discordgo.MessageEmbedField, 0, 15)
	if *pastas.Count < 1 {
		guildPastaList = append(guildPastaList, &discordgo.MessageEmbedField{
			Name:   "That's all folks!",
			Value:  "You've either reached the end of the list or there are no copypastas.",
			Inline: true,
		})
	}
	for _, v := range pastas.Items {
		preview := *v["pasta"].S
		if len(preview) > 50 {
			preview = preview[:50]
		}
		guildPastaList = append(guildPastaList, &discordgo.MessageEmbedField{
			Name:   *v["alias"].S,
			Value:  "Preview: " + preview,
			Inline: true,
		})
	}
	return guildPastaList[:len(guildPastaList)]
}

func sendPastaPage(discordSession *discordgo.Session, channelID string, pastas []*discordgo.MessageEmbedField) {
	discordSession.ChannelMessageSendEmbed(channelID,
		&discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{},
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: "https://cdn.discordapp.com/avatars/518879406509391878/ca293c592d560f09d958e85166938e88.png?size=256",
			},
			Color:       0x0000ff,
			Description: "It's not like I like you or a-anything, b-b-baka.",
			Fields:      pastas,
			Title:       "A list of your copypastas",
		})
}
