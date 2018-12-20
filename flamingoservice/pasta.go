package flamingoservice

import (
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
	"github.com/njha7/FlamingoV2/flamingolog"
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
		PastaServiceLogger: flamingolog.BuildServiceLogger(pastaServiceName),
		PastaErrorLogger:   flamingolog.BuildServiceErrorLogger(pastaServiceName),
	}
}

func (pastaClient *PastaClient) IsCommand(message string) bool {
	return false
}
func (pastaClient *PastaClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	return
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

func (pastaClient *PastaClient) EditPasta(guildID, channelID, requester, alias, pasta string) {
	_, err := pastaClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:           aws.String(pastaTableName),
		Key:                 buildPastaKey(guildID, alias),
		ConditionExpression: aws.String("#o=:r"),
		ExpressionAttributeNames: map[string]*string{
			"#o": aws.String("owner"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":p": &dynamodb.AttributeValue{S: aws.String(pasta)},
			":r": &dynamodb.AttributeValue{S: aws.String(requester)},
		},
		UpdateExpression: aws.String("SET pasta=:p"),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				author, err := pastaClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
					TableName:            aws.String(pastaTableName),
					Key:                  buildPastaKey(guildID, alias),
					ProjectionExpression: aws.String("#o"),
					ExpressionAttributeNames: map[string]*string{
						"#o": aws.String("owner"),
					},
				})
				if err != nil {
					pastaClient.PastaErrorLogger.Println(err)
					pastaClient.DiscordSession.ChannelMessageSend(channelID, "Only the author can update this pasta.")
				} else {
					authorID, ok := author.Item["owner"]
					if ok {
						pastaClient.DiscordSession.ChannelMessageSend(channelID, "Only <@"+*authorID.S+"> can update this pasta.")
					} else {
						pastaClient.DiscordSession.ChannelMessageSend(channelID, "Cannot update copypasta that does not exist. Please save first and try again.")
					}
				}
				return
			}
		}
		pastaClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Please try again later.")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}
	pastaClient.DiscordSession.ChannelMessageSend(channelID, "Copypasta with alias "+alias+" updated.")
}

func (pastaClient *PastaClient) ListPasta(guildID, channelID, userID string) {
	var guildName string
	guild, err := pastaClient.DiscordSession.Guild(guildID)
	if err != nil {
		guildName = "An error occurred while retrieving server name."
		pastaClient.PastaErrorLogger.Println(err)
	} else {
		guildName = guild.Name
	}

	dmChannel, err := pastaClient.DiscordSession.UserChannelCreate(userID)
	if err != nil {
		pastaClient.DiscordSession.ChannelMessageSend(channelID, "An error occured. Could not DM <@"+userID+">")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}

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

	err = pastaClient.DynamoClient.QueryPages(pastaList,
		func(page *dynamodb.QueryOutput, lastPage bool) bool {
			//List pastas in chat
			guildPastaList := buildPastaPage(page)
			pastaClient.DiscordSession.ChannelMessageSendEmbed(dmChannel.ID,
				&discordgo.MessageEmbed{
					Author: &discordgo.MessageEmbedAuthor{},
					Thumbnail: &discordgo.MessageEmbedThumbnail{
						URL: "https://cdn.discordapp.com/avatars/518879406509391878/ca293c592d560f09d958e85166938e88.png?size=256",
					},
					Color:       0x0000ff,
					Description: "It's not like I like you or a-anything, b-b-baka.",
					Fields:      guildPastaList,
					Title:       "Copypastas in " + guildName,
				})
			if lastPage {
				return false
			}
			return true
		})
	if err != nil {
		pastaClient.DiscordSession.ChannelMessageSend(dmChannel.ID, "An error occured. Please try again later.")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}
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
