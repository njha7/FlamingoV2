package flamingoservice

import (
	"log"
	"sort"
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
	MetricsClient     *flamingolog.FlamingoMetricsClient
	AuthServiceLogger *log.Logger
	AuthErrorLogger   *log.Logger
}

// Permission is an evaluatable representation of permission
type Permission struct {
	Guild   string
	Role    string
	Command string
	Action  string
	Allow   bool
}

// PermissionObject represents the schema of permissions
type PermissionObject struct {
	Guild      string `dynamodbav:"guild"`
	Permission string `dynamodbav:"perm"`
	Allow      bool   `dynamodbav:"allow"`
}

// PermissionKey is a convenience struct for marshalling Go types into DDB types
type PermissionKey struct {
	Guild      string `dynamodbav:"guild"`
	Permission string `dynamodbav:"perm"`
}

// NewAuthClient constructs an AuthClient
func NewAuthClient(discordClient *discordgo.Session,
	dynamoClient *dynamodb.DynamoDB,
	metricsClient *flamingolog.FlamingoMetricsClient) *AuthClient {
	return &AuthClient{
		DiscordClient:     discordClient,
		DynamoClient:      dynamoClient,
		MetricsClient:     metricsClient,
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
	guildRoles, err := authClient.DiscordClient.GuildRoles(guildID)
	if err != nil {
		authClient.AuthErrorLogger.Println(err)
		return false
	}
	//Track which roles a member has
	memberRoleSet := make(map[string]bool)
	//Track positions of roles
	rolePositionMap := make(map[int]string)

	for _, role := range roleIDList {
		memberRoleSet[role] = true
	}
	for _, role := range guildRoles {
		_, ok := memberRoleSet[role.ID]
		//Only populate position role map for roles that user has
		if ok {
			rolePositionMap[role.Position] = role.ID
		}
	}

	rolePositions := make([]int, 10)
	for position := range rolePositionMap {
		rolePositions = append(rolePositions, position)
	}
	//Sorted list of roles (asc)
	sort.Ints(rolePositions)

	rolePermissions := make(map[string]map[string]bool)
	userPermissions := make(map[string]map[string]bool)

	//TODO Query for permissiveness flag
	//Permissiveness flag defines behavior when no permissions records are found
	//permissive=true allows treats total absence permissions records for as a record granting permission
	//conversely, permissive=false treats a total absence as a record denying permission
	//if this record is missing, deny all requests

	authClient.DynamoClient.BatchGetItemPages(&dynamodb.BatchGetItemInput{
		RequestItems: buildAuthorizationKeys(guildID, userID, command, action, roleIDList),
	},
		func(page *dynamodb.BatchGetItemOutput, lastPage bool) bool {
			for _, permission := range page.Responses[assets.AuthTableName] {
				rule := &PermissionObject{}
				dynamodbattribute.UnmarshalMap(permission, rule)
				//Role-based rules
				if strings.HasSuffix(rule.Permission, "role") {
					//populate role permissions
				} else {
					//User-based rules
					//populate user permissions
				}
			}
			return !lastPage
		})
	/*
		TODO: first evaluate user permissions,
	*/
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
		keys = append(keys, buildAuthorizationKey(guildID, userID, role, command, "", true))
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
