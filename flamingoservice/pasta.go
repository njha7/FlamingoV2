package flamingoservice

import (
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
	"github.com/njha7/FlamingoV2/assets"
	"github.com/njha7/FlamingoV2/flamingolog"
)

const (
	pastaServiceName = "Pasta"
)

// PastaClient is responsible for handling "pasta" commands
type PastaClient struct {
	DynamoClient       *dynamodb.DynamoDB
	MetricsClient      *flamingolog.FlamingoMetricsClient
	PastaServiceLogger *log.Logger
	PastaErrorLogger   *log.Logger
}

// Pasta represents the schema of a pasta stored in DDB
type Pasta struct {
	Guild string `dynamodbav:"guild"`
	Alias string `dynamodbav:"alias"`
	Owner string `dynamodbav:"owner"`
	Pasta string `dynamodbav:"pasta"`
}

// PastaKey is a convenience struct for marshaling Go types into a key for DDB requests for a given pasta
type PastaKey struct {
	Guild string `dynamodbav:"guild"`
	Alias string `dynamodbav:"alias"`
}

// NewPastaClient constructs a PastaClient
func NewPastaClient(dynamoClient *dynamodb.DynamoDB, metricsClient *flamingolog.FlamingoMetricsClient) *PastaClient {
	return &PastaClient{
		DynamoClient:       dynamoClient,
		MetricsClient:      metricsClient,
		PastaServiceLogger: flamingolog.BuildServiceLogger(pastaServiceName),
		PastaErrorLogger:   flamingolog.BuildServiceErrorLogger(pastaServiceName),
	}
}

// IsCommand identifies a message as a potential command
func (pastaClient *PastaClient) IsCommand(message string) bool {
	return strings.HasPrefix(message, "pasta")
}

// Handle parses a command message and performs the commanded action
func (pastaClient *PastaClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	//first word is always "pasta", safe to remove
	args := strings.SplitN(message.Content, " ", 4)[1:]
	if len(args) < 1 {
		pastaClient.Help(session, message.ChannelID)
		return
	}
	//sub-commands of pasta
	switch args[0] {
	case "get":
		if len(args) < 2 {
			session.ChannelMessageSend(message.ChannelID, "Please specify a copypasta to get!")
			return
		}
		pasta, err := pastaClient.GetPasta(message.GuildID, args[1])
		ParseServiceResponse(session, message.ChannelID, pasta, err)
	case "save":
		if len(args) < 3 {
			session.ChannelMessageSend(message.ChannelID, "Please specify a copypasta or an alias!")
			return
		}
		result, err := pastaClient.SavePasta(message.GuildID, message.Author.ID, args[1], args[2])
		if result {
			ParseServiceResponse(session, message.ChannelID, "Copypasta with alias "+args[1]+" saved.", err)
		} else {
			ParseServiceResponse(session, message.ChannelID, "Copypasta with alias "+args[1]+" already exists.", err)
		}
	case "edit":
		if len(args) < 3 {
			session.ChannelMessageSend(message.ChannelID, "Please specify a copypasta or an alias!")
			return
		}
		result, err := pastaClient.EditPasta(message.GuildID, message.ChannelID, message.Author.ID, args[1], args[2])
		ParseServiceResponse(session, message.ChannelID, result, err)
	case "list":
		pastaClient.ListPasta(session, message.GuildID, message.ChannelID, message.Author.ID)
	case "help":
		pastaClient.Help(session, message.ChannelID)
	default:
		pastaClient.Help(session, message.ChannelID)
	}
}

// GetPasta returns a guild pasta by alias
func (pastaClient *PastaClient) GetPasta(guildID, alias string) (string, error) {
	result, err := pastaClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(assets.PastaTableName),
		Key:       buildPastaKey(guildID, alias),
	})
	if err != nil {
		pastaClient.PastaErrorLogger.Println(err)
		return "", err
	}
	pasta, ok := result.Item["pasta"]
	if ok {
		return *pasta.S, nil
	}
	return "No copypasta with alias " + alias + " found.", nil
}

// SavePasta saves a pasta, with a unique alias for a guild
func (pastaClient *PastaClient) SavePasta(guildID, owner, alias, pasta string) (bool, error) {
	_, err := pastaClient.DynamoClient.PutItem(&dynamodb.PutItemInput{
		TableName:           aws.String(assets.PastaTableName),
		Item:                buildPasta(guildID, owner, alias, pasta),
		ConditionExpression: aws.String("attribute_not_exists(guild) and attribute_not_exists(alias)"),
	})
	if err != nil {
		pastaClient.PastaErrorLogger.Println(err)
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

// EditPasta updates an existing pasta, provided the requester is the author of said pasta
func (pastaClient *PastaClient) EditPasta(guildID, channelID, requester, alias, pasta string) (string, error) {
	_, err := pastaClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:           aws.String(assets.PastaTableName),
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
					TableName:            aws.String(assets.PastaTableName),
					Key:                  buildPastaKey(guildID, alias),
					ProjectionExpression: aws.String("#o"),
					ExpressionAttributeNames: map[string]*string{
						"#o": aws.String("owner"),
					},
				})
				if err != nil {
					pastaClient.PastaErrorLogger.Println(err)
					return "Only the author can update this pasta.", nil
				}
				authorID, ok := author.Item["owner"]
				if ok {
					return "Only <@" + *authorID.S + "> can update this pasta.", nil
				}
				return "Cannot update copypasta that does not exist. Please save first and try again.", nil
			}
		}
		pastaClient.PastaErrorLogger.Println(err)
		return "", err
	}
	return "Copypasta with alias " + alias + " updated.", nil
}

// ListPasta dms the user a list of all pasta saved on the server it was called from
func (pastaClient *PastaClient) ListPasta(session *discordgo.Session, guildID, channelID, userID string) {
	var guildName string
	guild, err := session.Guild(guildID)
	if err != nil {
		guildName = "An error occurred while retrieving server name."
		pastaClient.PastaErrorLogger.Println(err)
	} else {
		guildName = guild.Name
	}

	dmChannel, err := session.UserChannelCreate(userID)
	if err != nil {
		session.ChannelMessageSend(channelID, "An error occured. Could not DM <@"+userID+">")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}

	pastaList := &dynamodb.QueryInput{
		TableName:              aws.String(assets.PastaTableName),
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
			session.ChannelMessageSendEmbed(dmChannel.ID,
				&discordgo.MessageEmbed{
					Author: &discordgo.MessageEmbedAuthor{},
					Thumbnail: &discordgo.MessageEmbedThumbnail{
						URL: assets.AvatarURL,
					},
					Color:       0x0000ff,
					Description: "It's not like I like you or a-anything, b-b-baka.",
					Fields:      guildPastaList,
					Title:       "Copypastas in " + guildName,
				})
			return !lastPage
		})
	if err != nil {
		session.ChannelMessageSend(dmChannel.ID, "An error occured. Please try again later.")
		pastaClient.PastaErrorLogger.Println(err)
		return
	}
}

// Help provides assistance with the pasta command by sending a help dialogue
func (pastaClient *PastaClient) Help(session *discordgo.Session, channelID string) {
	session.ChannelMessageSendEmbed(channelID,
		&discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{},
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: assets.AvatarURL,
			},
			Color:       0xff0000,
			Title:       "You need help!",
			Description: "The commands for pasta are:",
			Fields: []*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name: "get",
					Value: "Retrieves a copypasta by alias and posts it. Alias can by any alphanumeric string with no whitespace.\n" +
						"Usage: ```~pasta get $alias```",
				},
				&discordgo.MessageEmbedField{
					Name: "save",
					Value: "Saves a new a copypasta by alias. Alias can by any alphanumeric string with no whitespace.\n" +
						"Usage: ```~pasta save $alias $copypasta_text```",
				},
				&discordgo.MessageEmbedField{
					Name: "edit",
					Value: "Updates an existing copypasta by alias. The copypasta must exist and by authored by the caller for this to succeed.\n" +
						"Usage: ```~pasta save $alias $updated_copypasta_text```",
				},
				&discordgo.MessageEmbedField{
					Name: "list",
					Value: "Retrieves a paginated list of all the copypastas saved in the server and DMs them to the caller.\n" +
						"Usage: ```~pasta list```",
				},
				&discordgo.MessageEmbedField{
					Name:  "help",
					Value: "Shows this help message.",
				},
			},
		})
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
			Name:  "That's all folks!",
			Value: "You've either reached the end of the list or there are no copypastas.",
		})
	}
	for _, v := range pastas.Items {
		preview := *v["pasta"].S
		if len(preview) > 50 {
			preview = preview[:50]
		}
		guildPastaList = append(guildPastaList, &discordgo.MessageEmbedField{
			Name:  *v["alias"].S,
			Value: "Preview: " + preview,
		})
	}
	return guildPastaList[:len(guildPastaList)]
}
