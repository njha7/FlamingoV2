package flamingoservice

import (
	"FlamingoV2/assets"
	"FlamingoV2/flamingolog"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/bwmarrin/discordgo"
)

const (
	templateServiceName = "Template"
	templateCommand     = "template"
)

// TemplateClient is responsible for identifying and handling template commands
type TemplateClient struct {
	DynamoClient          *dynamodb.DynamoDB
	MetricsClient         *flamingolog.FlamingoMetricsClient
	AuthClient            *AuthClient
	TemplateServiceLogger *log.Logger
	TemplateErrorLogger   *log.Logger
}

// Template represents the schema of a template input stored in DDB
type Template struct {
	Guild    string `dynamodbav:"guild"`
	Alias    string `dynamodbav:"alias"`
	Owner    string `dynamodbav:"owner"`
	Template string `dynamodbav:"template"`
}

type TemplateKey struct {
	Guild string `dynamodbav:"guild"`
	Alias string `dynamodbav:"alias"`
}

func NewTemplateClient(dynamoClient *dynamodb.DynamoDB, metricsClient *flamingolog.FlamingoMetricsClient, authClient *AuthClient) *TemplateClient {
	return &TemplateClient{
		DynamoClient:          dynamoClient,
		MetricsClient:         metricsClient,
		AuthClient:            authClient,
		TemplateServiceLogger: flamingolog.BuildServiceLogger(templateServiceName),
		TemplateErrorLogger:   flamingolog.BuildServiceErrorLogger(templateServiceName),
	}
}

func (templateClient *TemplateClient) IsCommand(message string) bool {
	return strings.HasPrefix(message, templateCommand)
}

func (templateClient *TemplateClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	args := strings.SplitN(message.Content, " ", 4)[1:]
	if len(args) < 1 {
		templateClient.Help(session, message.ChannelID)
		return
	}

	switch args[0] {
	case "get":
		// 1 value means they only passed us ~template get
		if len(args) < 2 {
			_, err := session.ChannelMessageSend(message.ChannelID, "Please specify a template to get!")
			if err != nil {
				session.ChannelMessageSend(message.ChannelID, "Template retrieval failed, please try later!")
				templateClient.TemplateErrorLogger.Println(err)
			}
			return
		}

		// 2 values means they only passed us ~template get <template>, but no substitution value
		if len(args) < 3 {
			_, err := session.ChannelMessageSend(message.ChannelID, "Please specify a substitution value!")
			if err != nil {
				session.ChannelMessageSend(message.ChannelID, "Template retrieval failed, please try later!")
				templateClient.TemplateErrorLogger.Println(err)
			}
		}

		if templateClient.AuthClient.Authorize(message.GuildID, message.Author.ID, templateCommand, "get") {
			template, err := templateClient.GetTemplate(message.GuildID, args[1], args[2])
			ParseServiceResponse(session, message.ChannelID, template, err)
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "save":
		if len(args) < 3 {
			_, err := session.ChannelMessageSend(message.ChannelID, "Please specify a template alias or a template!")
			if err != nil {
				session.ChannelMessageSend(message.ChannelID, "Template save failed, please try later!")
				templateClient.TemplateErrorLogger.Println(err)
			}
			return
		}

		// No %s, no dice.
		if !strings.Contains(args[2], "%s") {
			_, err := session.ChannelMessageSend(message.ChannelID, "Yo, dimwit. You need to specify where I need to sub stuff! Add a '%s'")
			if err != nil {
				session.ChannelMessageSend(message.ChannelID, "Template save failed, please try later!")
				templateClient.TemplateErrorLogger.Println(err)
			}
			return
		}

		if templateClient.AuthClient.Authorize(message.GuildID, message.Author.ID, templateCommand, "get") {
			result, err := templateClient.SaveTemplate(message.GuildID, message.Author.ID, args[1], args[2])
			if result {
				ParseServiceResponse(session, message.ChannelID, "Template with alias "+args[1]+" saved.", err)
			} else {
				ParseServiceResponse(session, message.ChannelID, "Template with alias "+args[1]+" already exists.", err)
			}
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "edit":
		if len(args) < 3 {
			_, err := session.ChannelMessageSend(message.ChannelID, "Please specify a template or alias!")
			if err != nil {
				session.ChannelMessageSend(message.ChannelID, "Template edit failed, please try later!")
				templateClient.TemplateErrorLogger.Println(err)
			}
			return
		}

		if !strings.Contains(args[2], "%s") {
			_, err := session.ChannelMessageSend(message.ChannelID, "Yo, dimwit. You need to specify where I need to sub stuff! Add a '%s'")
			if err != nil {
				session.ChannelMessageSend(message.ChannelID, "Template edit failed, please try later!")
				templateClient.TemplateErrorLogger.Println(err)
			}
			return
		}

		result, err := templateClient.EditTemplate(message.GuildID, message.ChannelID, message.Author.ID, args[1], args[2])
		ParseServiceResponse(session, message.ChannelID, result, err)
	case "list":
		templateClient.ListTemplate(session, message.GuildID, message.ChannelID, message.Author.ID)
	case "help":
		templateClient.Help(session, message.ChannelID)
	default:
		templateClient.Help(session, message.ChannelID)
	}
}

func (templateClient *TemplateClient) Help(session *discordgo.Session, channelID string) {
	_, err := session.ChannelMessageSendEmbed(channelID,
		&discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{},
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: assets.AvatarURL,
			},
			Color:       0xff0000,
			Title:       "Someone called a waaambulance!",
			Description: "The commands for template are: ",
			Fields: []*discordgo.MessageEmbedField{
				{
					Name: "get",
					Value: "Retrieves a template by alias and substitutes the given string. Alias can be any alphanumeric string with no whitespace.\n" +
						"Usage: ```~template get $alias $substitute```",
				},
				{
					Name: "save",
					Value: "Saves a new template by alias. Alias can be any alphanumeric string with no whitespace. Must include a %s substitute.\n" +
						"Usage: ```~template save $alias $template```",
				},
				{
					Name: "edit",
					Value: "Updates an existing template by alias. The alias must exist and be authored by the caller for this to succeed.\n" +
						"Usage: ```~template edit $alias $new_template```",
				},
				{
					Name: "list",
					Value: "Retrieves a paginated list of templates saved to the current server and DMs them to the caller.\n" +
						"Usage: ```~template list```",
				},
				{
					Name:  "help",
					Value: "Shows this text.",
				},
			},
		})
	if err != nil {
		session.ChannelMessageSend(channelID, "Something broke with your help message. Please try again!")
		templateClient.TemplateErrorLogger.Println(err)
		return
	}
}

func (templateClient *TemplateClient) SaveTemplate(guildID, owner, alias, template string) (bool, error) {
	_, err := templateClient.DynamoClient.PutItem(&dynamodb.PutItemInput{
		TableName:           aws.String(assets.PastaTableName),
		Item:                buildTemplate(guildID, owner, alias, template),
		ConditionExpression: aws.String("attribute_not_exists(guild) and attribute_not_exists(alias)"),
	})
	if err != nil {
		templateClient.TemplateErrorLogger.Println(err)
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

func buildTemplate(guildID, owner, alias, template string) map[string]*dynamodb.AttributeValue {
	templateItem := Template{
		Guild:    guildID + "T",
		Alias:    alias,
		Owner:    owner,
		Template: template,
	}
	// Similar to Pasta, err != nil will get caught in request
	item, _ := dynamodbattribute.MarshalMap(templateItem)
	return item
}

func (templateClient *TemplateClient) ListTemplate(session *discordgo.Session, guildID, channelID, userID string) {
	var guildName string
	guild, err := session.Guild(guildID)
	if err != nil {
		guildName = "An error occurred while retrieving server name."
		templateClient.TemplateErrorLogger.Println(err)
	} else {
		guildName = guild.Name
	}

	dmChannel, err := session.UserChannelCreate(userID)
	if err != nil {
		session.ChannelMessageSend(channelID, "An error occurred. Could not DM <@"+userID+">")
		templateClient.TemplateErrorLogger.Println(err)
		return
	}

	templateList := &dynamodb.QueryInput{
		TableName:              aws.String(assets.PastaTableName),
		KeyConditionExpression: aws.String("guild=:g"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":g": {
				S: aws.String(guildID + "T"),
			},
		},
		Limit: aws.Int64(15),
	}

	err = templateClient.DynamoClient.QueryPages(templateList,
		func(page *dynamodb.QueryOutput, lastPage bool) bool {
			//List templates in chat
			guildTemplateList := buildTemplatePage(page)
			session.ChannelMessageSendEmbed(dmChannel.ID,
				&discordgo.MessageEmbed{
					Author: &discordgo.MessageEmbedAuthor{},
					Thumbnail: &discordgo.MessageEmbedThumbnail{
						URL: assets.AvatarURL,
					},
					Color:       0x0000ff,
					Description: "It's not like I like you or a-anything, b-b-baka.",
					Fields:      guildTemplateList,
					Title:       "Templates in " + guildName,
				})
			return !lastPage
		})
	if err != nil {
		session.ChannelMessageSend(dmChannel.ID, "An error occured. Please try again later.")
		templateClient.TemplateErrorLogger.Println(err)
		return
	}
}

func buildTemplatePage(templates *dynamodb.QueryOutput) []*discordgo.MessageEmbedField {
	//List templates in chat
	guildTemplateList := make([]*discordgo.MessageEmbedField, 0, 15)
	if *templates.Count < 1 {
		guildTemplateList = append(guildTemplateList, &discordgo.MessageEmbedField{
			Name:  "That's all folks!",
			Value: "You've either reached the end of the list or there are no templates.",
		})
	}
	for _, v := range templates.Items {
		preview := *v["template"].S
		if len(preview) > 50 {
			preview = preview[:50]
		}
		guildTemplateList = append(guildTemplateList, &discordgo.MessageEmbedField{
			Name:  *v["alias"].S,
			Value: "Preview: " + preview,
		})
	}
	return guildTemplateList[:]
}

func (templateClient *TemplateClient) EditTemplate(guildID, channelID, requester, alias, template string) (string, error) {
	_, err := templateClient.DynamoClient.UpdateItem(&dynamodb.UpdateItemInput{
		TableName:           aws.String(assets.PastaTableName),
		Key:                 buildTemplateKey(guildID, alias),
		ConditionExpression: aws.String("#o=:r"),
		ExpressionAttributeNames: map[string]*string{
			"#o": aws.String("owner"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":t": {S: aws.String(template)},
			":r": {S: aws.String(requester)},
		},
		UpdateExpression: aws.String("SET template=:t"),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				author, err := templateClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
					TableName:            aws.String(assets.PastaTableName),
					Key:                  buildTemplateKey(guildID, alias),
					ProjectionExpression: aws.String("#o"),
					ExpressionAttributeNames: map[string]*string{
						"#o": aws.String("owner"),
					},
				})
				if err != nil {
					templateClient.TemplateErrorLogger.Println(err)
					return "Only the author can update this template.", nil
				}
				authorID, ok := author.Item["owner"]
				if ok {
					return fmt.Sprintf("Only <@%s> can update this template.", *authorID.S), nil
				}
				return "Cannot update template that does not exist. Please save first and try again.", nil
			}
		}
		templateClient.TemplateErrorLogger.Println(err)
		return "", err
	}
	return fmt.Sprintf("Template with alias %s updated.", alias), nil
}

func (templateClient *TemplateClient) GetTemplate(guildID, alias, sub string) (string, error) {
	result, err := templateClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(assets.PastaTableName),
		Key:       buildTemplateKey(guildID, alias),
	})

	if err != nil {
		templateClient.TemplateErrorLogger.Println(err)
		return "", err
	}

	template, ok := result.Item["template"]
	if ok {
		return fmt.Sprintf(*template.S, sub), nil
	}

	return fmt.Sprintf("No template with alias %s found", alias), nil
}

func buildTemplateKey(guildID, alias string) map[string]*dynamodb.AttributeValue {
	id := TemplateKey{
		Guild: guildID + "T",
		Alias: alias,
	}

	key, _ := dynamodbattribute.MarshalMap(id)
	return key
}
