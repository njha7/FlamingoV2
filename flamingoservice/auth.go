package flamingoservice

import (
	"errors"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"

	"github.com/njha7/FlamingoV2/assets"

	"github.com/bwmarrin/discordgo"

	"github.com/njha7/FlamingoV2/flamingolog"

	"github.com/aws/aws-sdk-go/service/dynamodb"
)

const (
	authServiceName = "Auth"
)

var (
	command, _         = regexp.Compile(`command=\w*`)
	action, _          = regexp.Compile(`action=\w*`)
	user, _            = regexp.Compile(`user=\s?\<\@![\d]*\>`)
	role, _            = regexp.Compile(`role=\s?\<\@\&[\d]*\>`)
	permissionValue, _ = regexp.Compile(`permission=(true|false)`)
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

// Handle parses a command message and performs the commanded action
func (authClient *AuthClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	//first word is always "auth", safe to remove
	args := strings.SplitN(message.Content, " ", 3)[1:]
	if len(args) < 1 {
		authClient.Help(session, message.ChannelID)
		return
	}
	authClient.AuthServiceLogger.Println(message.Content)
	//sub-commands of auth
	switch args[0] {
	case "set":
		commandPermission, actionPermission, _, _, isRole, isAllowed := parseAuthCommandArgs(message.Content)
		if isRole {
			authClient.SetPermission(message.GuildID, message.MentionRoles[0], commandPermission, actionPermission, isRole, isAllowed)
		} else {
			authClient.SetPermission(message.GuildID, message.Mentions[0].ID, commandPermission, actionPermission, isRole, isAllowed)
		}
	case "delete":
		commandPermission, actionPermission, _, _, isRole, _ := parseAuthCommandArgs(message.Content)
		if isRole {
			authClient.DeletePermission(message.GuildID, message.MentionRoles[0], commandPermission, actionPermission, isRole)
		} else {
			authClient.DeletePermission(message.GuildID, message.Mentions[0].ID, commandPermission, actionPermission, isRole)
		}
	case "help":
		authClient.Help(session, message.ChannelID)
	}
}

// SetPermission sets the value of a permission
func (authClient *AuthClient) SetPermission(guildID, ID, command, action string, isRole, isAllowed bool) (bool, error) {
	permission := make(map[string]*dynamodb.AttributeValue)
	if isRole {
		permission = buildPermission(guildID, "", ID, command, action, isRole, isAllowed)
	} else {
		permission = buildPermission(guildID, ID, "", command, action, isRole, isAllowed)
	}
	_, err := authClient.DynamoClient.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(assets.AuthTableName),
		Item:      permission,
	})
	if err != nil {
		authClient.AuthErrorLogger.Println(err)
		return false, err
	}
	return true, nil
}

// DeletePermission deletes the records associated with a permission
func (authClient *AuthClient) DeletePermission(guildID, ID, command, action string, isRole bool) (bool, error) {
	var key map[string]*dynamodb.AttributeValue
	if isRole {
		key = buildAuthorizationKey(guildID, "", ID, command, action, isRole)
	} else {
		key = buildAuthorizationKey(guildID, ID, "", command, action, isRole)
	}
	_, err := authClient.DynamoClient.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(assets.AuthTableName),
		Key:       key,
	})
	if err != nil {
		authClient.AuthErrorLogger.Println(err)
		return false, err
	}
	return true, nil
}

//Authorize determines a user's eligibility to invoke a command
// returns true if authorized, false otherwise
func (authClient *AuthClient) Authorize(guildID, userID, command, action string) bool {
	//Commands should always work in dms
	if guildID == "" {
		return true
	}
	//Check permissive flag value
	permissiveFlagValue, err := authClient.GetPermissiveFlagValue(guildID)
	if err != nil {
		authClient.AuthErrorLogger.Println(err)
		return false
	}

	member, err := authClient.DiscordClient.GuildMember(guildID, userID)
	if err != nil {
		authClient.AuthErrorLogger.Println(err)
		return false
	}
	roleIDList := member.Roles

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
	userPermissions := make(map[string]bool)

	authClient.DynamoClient.BatchGetItemPages(&dynamodb.BatchGetItemInput{
		RequestItems: buildAuthorizationKeys(guildID, userID, command, action, roleIDList),
	},
		func(page *dynamodb.BatchGetItemOutput, lastPage bool) bool {
			for _, permission := range page.Responses[assets.AuthTableName] {
				rule := &PermissionObject{}
				dynamodbattribute.UnmarshalMap(permission, rule)
				//Role-based rules
				if strings.HasPrefix(rule.Permission, "role!") {
					//guild, command ! action
					ruleArgs := strings.SplitN(rule.Guild, "!", 2)
					roleID := strings.Split(rule.Permission, "!")[1]
					//populate role permissions
					_, ok := rolePermissions[roleID]
					if !ok {
						rolePermissions[roleID] = make(map[string]bool)
					}
					rolePermissions[roleID][ruleArgs[1]] = rule.Allow
				} else {
					//User-based rules
					//guild, command ! action
					ruleArgs := strings.SplitN(rule.Guild, "!", 2)
					//populate user permissions
					userPermissions[ruleArgs[1]] = rule.Allow
				}
			}
			return !lastPage
		})
	//Do short circuit check
	if len(rolePermissions) == 0 && len(userPermissions) == 0 {
		//auth command requires explicit permission to execute
		return permissiveFlagValue && command != "auth"
	}
	//Evaluate user permissions
	userPerm := evaluatePermissions(userPermissions, command, action)
	if userPerm != nil {
		return *userPerm
	}
	for i := len(rolePositions) - 1; i > 0; i-- {
		rolePerm, ok := rolePermissions[rolePositionMap[rolePositions[i]]]
		if ok {
			//Impossible for this to be nil, will return T or F
			return *evaluatePermissions(rolePerm, command, action)
		}
	}
	return false
}

// GetPermissiveFlagValue checks for the value of the permissive flag for a guild.
func (authClient *AuthClient) GetPermissiveFlagValue(guildID string) (bool, error) {
	//Permissiveness flag defines behavior when no permissions records are found
	//permissive=true allows treats total absence permissions records for as a record granting permission
	//conversely, permissive=false treats a total absence as a record denying permission
	//if this record is missing, deny all requests
	result, err := authClient.DynamoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(assets.AuthTableName),
		Key:       buildAuthorizationKey(guildID, "", "", "", "", false),
	})
	if err != nil {
		authClient.AuthErrorLogger.Println(err)
		return false, err
	}
	_, ok := result.Item["perm"]
	if !ok {
		return false, errors.New("Permissive flag not found for guild:" + guildID)
	}
	permissiveFlag := &PermissionObject{}
	dynamodbattribute.UnmarshalMap(result.Item, permissiveFlag)
	return permissiveFlag.Allow, nil
}

// SetDefaultPermissiveFlagValue sets the value of the permissiveness flag to true for the first time
func (authClient *AuthClient) SetDefaultPermissiveFlagValue(guildID string) error {
	//Permissiveness flag defines behavior when no permissions records are found
	//permissive=true allows treats total absence permissions records for as a record granting permission
	//conversely, permissive=false treats a total absence as a record denying permission
	_, err := authClient.DynamoClient.PutItem(&dynamodb.PutItemInput{
		TableName:           aws.String(assets.AuthTableName),
		Item:                buildPermission(guildID, "", "", "", "", false, true),
		ConditionExpression: aws.String("attribute_not_exists(guild) and attribute_not_exists(perm)"),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				return nil
			default:
				authClient.AuthErrorLogger.Println(err)
				return err
			}
		}
		authClient.AuthErrorLogger.Println(err)
		return err
	}
	return nil
}

// Help provides assistance with the auth command by sending a help dialogue
func (authClient *AuthClient) Help(session *discordgo.Session, channelID string) {
	session.ChannelMessageSendEmbed(channelID,
		&discordgo.MessageEmbed{
			Author: &discordgo.MessageEmbedAuthor{},
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: assets.AvatarURL,
			},
			Color:       0xff0000,
			Title:       "You need help!",
			Description: "The commands for auth are:",
			Fields: []*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name: "set",
					Value: "Creates a permission rule for a given command and user or role\n" +
						"Usage: ~auth set command=$command *action=$action ^user=@user ^role=@role permission=$bool\n" +
						"* - optional argument\n" +
						"^ - XOR",
				},
				&discordgo.MessageEmbedField{
					Name: "delete",
					Value: "Deletes a permission rule for a given command and user or role\n" +
						"Usage: ~auth delete command=$command *action=$action ^user=@user ^role=@role permission=$bool\n" +
						"* - optional argument\n" +
						"^ - XOR",
				},
			},
		})
}

func parseAuthCommandArgs(message string) (commandPermission, actionPermission, userPermission, roleIDPermission string, isRole, isAllowed bool) {
	commandPermission = command.FindString(message)
	actionPermission = action.FindString(message)
	userPermission = user.FindString(message)
	roleIDPermission = role.FindString(message)
	isRole = false
	switch permissionValue.FindString(message) {
	case "permission=true":
		isAllowed = true
	case "permission=false":
		isAllowed = false
	default:
		isAllowed = false
	}
	if commandPermission != "" {
		commandPermission = strings.Split(commandPermission, "=")[1]
	}
	if actionPermission != "" {
		actionPermission = strings.Split(actionPermission, "=")[1]
	}
	if userPermission == "" {
		isRole = true
	}
	return
}

func evaluatePermissions(permissions map[string]bool, command, action string) *bool {
	var hasPermission *bool
	commandPermission, cp := permissions[command+"!"]
	actionPermission, ap := permissions[command+"!"+action]
	if ap {
		return &actionPermission
	}
	if cp {
		return &commandPermission
	}
	return hasPermission
}

func buildAuthorizationKeys(guildID, userID, command, action string, roleIDList []string) map[string]*dynamodb.KeysAndAttributes {
	keysAndAttributes := map[string]*dynamodb.KeysAndAttributes{
		assets.AuthTableName: &dynamodb.KeysAndAttributes{},
	}

	keys := make([]map[string]*dynamodb.AttributeValue, 0, 10)
	//Construct keys for roles
	for _, role := range roleIDList {
		keys = append(keys, buildAuthorizationKey(guildID, userID, role, command, action, true))
		keys = append(keys, buildAuthorizationKey(guildID, userID, role, command, "", true))
	}
	//Add key for userID
	keys = append(keys, buildAuthorizationKey(guildID, userID, "", command, action, false))
	keys = append(keys, buildAuthorizationKey(guildID, userID, "", command, "", false))
	keysAndAttributes[assets.AuthTableName].SetKeys(keys[:len(keys)])
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

func buildPermission(guildID, userID, roleID, command, action string, isRole, isAllowed bool) map[string]*dynamodb.AttributeValue {
	var rangeKey string
	if isRole {
		rangeKey = "role!" + roleID
	} else {
		rangeKey = "user!" + userID
	}
	permission, _ := dynamodbattribute.MarshalMap(PermissionObject{
		Guild:      guildID + "!" + command + "!" + action,
		Permission: rangeKey,
		Allow:      isAllowed,
	})
	return permission
}
