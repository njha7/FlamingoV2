package flamingoservice

import (
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"

	"github.com/njha7/FlamingoV2/assets"

	"github.com/bwmarrin/discordgo"

	"github.com/njha7/FlamingoV2/flamingolog"

	"github.com/aws/aws-sdk-go/service/dynamodb"
)

const (
	authServiceName = "Auth"
)

// AuthClient is responsible for enforcing permissions and handling
// permssions update commands
type AuthClient struct {
	DiscordClient     *discordgo.Session
	DynamoClient      *dynamodb.DynamoDB
	AuthServiceLogger *log.Logger
	AuthErrorLogger   *log.Logger
}

// Permission represents the schema of permissions
type Permission struct {
	Command string
	RoleID  string
	UserID  string
	Allow   bool
}

// PermissionKey is a convenience struct for marshalling Go types into DDB types
type PermissionKey struct {
	Guild      string `dynamodbav:"guild"`
	Permission string `dynamodbav:"perm"`
}

// NewAuthClient constructs an AuthClient
func NewAuthClient(discordClient *discordgo.Session, dynamoClient *dynamodb.DynamoDB) *AuthClient {
	return &AuthClient{
		DiscordClient:     discordClient,
		DynamoClient:      dynamoClient,
		AuthServiceLogger: flamingolog.BuildServiceLogger(authServiceName),
		AuthErrorLogger:   flamingolog.BuildServiceErrorLogger(authServiceName),
	}
}

// IsCommand identifies a message as a potential command
func (authClient *AuthClient) IsCommand(message string) bool {
	return strings.HasPrefix(message, "auth")
}

//Authorize determines a user's eligibility to invoke a command
// returns true if authorized, false otherwise
func (authClient *AuthClient) Authorize(guildID, userID, command, action string, roleIDList []string) bool {
	_, err := authClient.DiscordClient.GuildRoles(guildID)
	if err != nil {
		authClient.AuthErrorLogger.Println(err)
		return false
	}

	authClient.DynamoClient.BatchGetItemPages(&dynamodb.BatchGetItemInput{
		RequestItems: buildAuthorizationKeys(guildID, userID, command, action, roleIDList),
	},
		func(page *dynamodb.BatchGetItemOutput, lastPage bool) bool {

			return false
		})
	return false
}

func buildAuthorizationKeys(guildID, userID, command, action string, roleIDList []string) map[string]*dynamodb.KeysAndAttributes {
	keysAndAttributes := map[string]*dynamodb.KeysAndAttributes{
		assets.AuthTableName: &dynamodb.KeysAndAttributes{},
	}

	keys := make([]map[string]*dynamodb.AttributeValue, 10)
	//Construct keys for roles
	for _, role := range roleIDList {
		keys = append(keys, buildAuthorizationKey(guildID, userID, role, command, action, true))
	}
	//Add key for userID
	keys = append(keys, buildAuthorizationKey(guildID, userID, "", command, action, false))
	keysAndAttributes[assets.AuthTableName].SetKeys(keys)
	return keysAndAttributes
}

func buildAuthorizationKey(guildID, userID, roleID, command, action string, isRole bool) map[string]*dynamodb.AttributeValue {
	var rangeKey string
	if isRole {
		rangeKey = "role!" + roleID
	} else {
		rangeKey = "user!" + userID
	}
	key, _ := dynamodbattribute.MarshalMap(PermissionKey{
		Guild:      guildID + "!" + command + "!" + action,
		Permission: rangeKey,
	})
	return key
}
