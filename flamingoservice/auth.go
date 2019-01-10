package flamingoservice

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"

	"github.com/njha7/FlamingoV2/assets"

	"github.com/bwmarrin/discordgo"

	"github.com/njha7/FlamingoV2/flamingolog"

	"github.com/aws/aws-sdk-go/service/dynamodb"
)

const (
	authServiceName = "Auth"
	authCommand     = "auth"
)

var (
	command, _         = regexp.Compile(`command=\w*`)
	action, _          = regexp.Compile(`action=\w*`)
	user, _            = regexp.Compile(`user=\s?\<\@\!?[\d]*\>`)
	role, _            = regexp.Compile(`role=\s?\<\@\&[\d]*\>`)
	roleName, _        = regexp.Compile(`roleName=".*"`)
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
	return strings.HasPrefix(message, authCommand)
}

// Handle parses a command message and performs the commanded action
func (authClient *AuthClient) Handle(session *discordgo.Session, message *discordgo.Message) {
	//first word is always "auth", safe to remove
	args := strings.SplitN(message.Content, " ", 3)[1:]
	if len(args) < 1 {
		authClient.Help(session, message.ChannelID)
		return
	}
	//sub-commands of auth
	switch args[0] {
	case "set":
		if authClient.Authorize(message.GuildID, message.Author.ID, authCommand, "set") {
			var err error
			commandPermission, actionPermission, userPermission, roleIDPermission, isRole, isAllowed := parseAuthCommandArgs(session, message)
			if !validatePermissionID(userPermission, roleIDPermission) {
				ParseServiceResponse(session, message.ChannelID, "Please specify a user or a role!", nil)
				return
			}
			if isRole {
				err = authClient.SetPermission(message.GuildID, message.MentionRoles[0], commandPermission, actionPermission, isRole, isAllowed)
			} else {
				err = authClient.SetPermission(message.GuildID, message.Mentions[0].ID, commandPermission, actionPermission, isRole, isAllowed)
			}
			ParseServiceResponse(session, message.ChannelID, "Auth rule added successfully", err)
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "delete":
		if authClient.Authorize(message.GuildID, message.Author.ID, authCommand, "delete") {
			var err error
			commandPermission, actionPermission, userPermission, roleIDPermission, isRole, _ := parseAuthCommandArgs(session, message)
			if !validatePermissionID(userPermission, roleIDPermission) {
				ParseServiceResponse(session, message.ChannelID, "Please specify a user or a role!", nil)
				return
			}
			if isRole {
				err = authClient.DeletePermission(message.GuildID, message.MentionRoles[0], commandPermission, actionPermission, isRole)
			} else {
				err = authClient.DeletePermission(message.GuildID, message.Mentions[0].ID, commandPermission, actionPermission, isRole)
			}
			ParseServiceResponse(session, message.ChannelID, "Auth rule removed successfully", err)
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "test":
		commandPermission, actionPermission, userPermission, _, _, _ := parseAuthCommandArgs(session, message)
		if !validatePermissionID(userPermission, userPermission) {
			ParseServiceResponse(session, message.ChannelID, "Please specify a user!", nil)
			return
		}
		testMessage := "Unauthorized"
		result := authClient.Authorize(message.GuildID, message.Mentions[0].ID, commandPermission, actionPermission)
		if result {
			testMessage = "Authorized"
		}
		ParseServiceResponse(session, message.ChannelID, testMessage, nil)
	case "permissive":
		if authClient.Authorize(message.GuildID, message.Author.ID, authCommand, "permissive") {
			_, _, _, _, _, isAllowed := parseAuthCommandArgs(session, message)
			err := authClient.SetPermissiveFlagValue(message.GuildID, isAllowed)
			ParseServiceResponse(session, message.ChannelID, "Permissive flag value updated.", err)
		} else {
			ParseServiceResponse(session, message.ChannelID, "<@"+message.Author.ID+"> is unauthorized to issue that command!", nil)
		}
	case "list":
		ParseServiceResponse(session, message.ChannelID, "Coming soon!", nil)
		// err := authClient.ListPermissions(message.GuildID, message.ChannelID, message.Author.ID)
		// if err != nil {
		// 	ParseServiceResponse(session, message.ChannelID, "", err)
		// }
	case "help":
		authClient.Help(session, message.ChannelID)
	}
}

// ListPermissions lists all permissions for a guild
// func (authClient *AuthClient) ListPermissions(guildID, channelID, userID string) error {
// 	var guildName string
// 	guild, err := authClient.DiscordClient.Guild(guildID)
// 	if err != nil {
// 		guildName = "An error occurred while retrieving server name."
// 		authClient.AuthErrorLogger.Println(err)
// 	} else {
// 		guildName = guild.Name
// 	}

// 	guildRoles, err := authClient.DiscordClient.GuildRoles(guildID)
// 	if err != nil {
// 		authClient.AuthErrorLogger.Println(err)
// 		return err
// 	}
// 	roleIDNameMap := make(map[string]string)
// 	for _, v := range guildRoles {
// 		roleIDNameMap[v.ID] = v.Name
// 	}

// 	dmChannel, err := authClient.DiscordClient.UserChannelCreate(userID)
// 	if err != nil {
// 		authClient.DiscordClient.ChannelMessageSend(channelID, "An error occured. Could not DM <@"+userID+">")
// 		authClient.AuthErrorLogger.Println(err)
// 		return err
// 	}

// 	err = authClient.DynamoClient.BatchGetItemPages(&dynamodb.BatchGetItemInput{
// 		RequestItems: listAuthorizationKeys(guildID),
// 	},
// 		func(page *dynamodb.BatchGetItemOutput, lastPage bool) bool {
// 			permissionsList := make([]*discordgo.MessageEmbedField, 0, 10)
// 			for _, permission := range page.Responses[assets.AuthTableName] {
// 				rule := &PermissionObject{}
// 				dynamodbattribute.UnmarshalMap(permission, rule)
// 				//guild, command ! action
// 				ID := strings.Split(rule.Permission, "!")[1]
// 				permissionString := "Denied"
// 				if rule.Allow {
// 					permissionString = "Allowed"
// 				}
// 				//Role-based rules
// 				if strings.HasPrefix(rule.Permission, "role!") {
// 					permissionsList = append(permissionsList, &discordgo.MessageEmbedField{
// 						Name:  roleIDNameMap[ID],
// 						Value: permissionString,
// 					})
// 				} else {
// 					var userName string
// 					member, err := authClient.DiscordClient.GuildMember(guildID, ID)
// 					if err != nil {
// 						userName = ID
// 					} else {
// 						userName = member.Nick
// 					}
// 					permissionsList = append(permissionsList, &discordgo.MessageEmbedField{
// 						Name:  userName,
// 						Value: permissionString,
// 					})
// 				}
// 				authClient.DiscordClient.ChannelMessageSendEmbed(dmChannel.ID,
// 					&discordgo.MessageEmbed{
// 						Author: &discordgo.MessageEmbedAuthor{},
// 						Thumbnail: &discordgo.MessageEmbedThumbnail{
// 							URL: assets.AvatarURL,
// 						},
// 						Color:       0x0000ff,
// 						Description: "It's not like I like you or a-anything, b-b-baka.",
// 						Fields:      permissionsList,
// 						Title:       "Permissions for " + guildName,
// 					})
// 			}
// 			return !lastPage
// 		})
// 	if err != nil {
// 		authClient.DiscordClient.ChannelMessageSend(dmChannel.ID, "An error occured. Please try again later.")
// 		authClient.AuthErrorLogger.Println(err)
// 		return nil
// 	}
// 	return nil
// }

// SetPermission sets the value of a permission
func (authClient *AuthClient) SetPermission(guildID, ID, command, action string, isRole, isAllowed bool) error {
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
	}
	return err
}

// DeletePermission deletes the records associated with a permission
func (authClient *AuthClient) DeletePermission(guildID, ID, command, action string, isRole bool) error {
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
	}
	return nil
}

//Authorize determines a user's eligibility to invoke a command
// returns true if authorized, false otherwise
func (authClient *AuthClient) Authorize(guildID, userID, command, action string) bool {
	fmt.Printf("guildID:%s userID:%s, command:%s, action:%s\n", guildID, userID, command, action)
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
			// fmt.Println(page)
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

// SetPermissiveFlagValue sets the value of the permissiveness flag to true for the first time
func (authClient *AuthClient) SetPermissiveFlagValue(guildID string, value bool) error {
	//Permissiveness flag defines behavior when no permissions records are found
	//permissive=true allows treats total absence permissions records for as a record granting permission
	//conversely, permissive=false treats a total absence as a record denying permission
	_, err := authClient.DynamoClient.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(assets.AuthTableName),
		Item:      buildPermission(guildID, "", "", "", "", false, value),
	})
	if err != nil {
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
						"Usage: ~auth set command=$command *action=$action ^user=@user ^role=@role ^roleName=\"roleName\" permission=$bool\n" +
						"* - optional argument\n" +
						"^ - XOR",
				},
				&discordgo.MessageEmbedField{
					Name: "delete",
					Value: "Deletes a permission rule for a given command and user or role\n" +
						"Usage: ~auth delete command=$command *action=$action ^user=@user ^role=@role ^roleName=\"roleName\" permission=$bool\n" +
						"* - optional argument\n" +
						"^ - XOR",
				},
				&discordgo.MessageEmbedField{
					Name: "permissive",
					Value: "Sets the value of the permissive flag\n" +
						"Usage: ~auth permissive permission=$bool\n",
				},
				&discordgo.MessageEmbedField{
					Name: "test",
					Value: "Tests a permission rule for a given command and user or role\n" +
						"Usage: ~auth delete command=$command *action=$action user=@user\n" +
						"* - optional argument\n",
				},
				&discordgo.MessageEmbedField{
					Name: "list",
					Value: "Lists all the permissions rules for the guild\n" +
						"Usage: ~auth list",
				},
			},
		})
}

func parseAuthCommandArgs(discordClient *discordgo.Session, message *discordgo.Message) (commandPermission, actionPermission, userPermission, roleIDPermission string, isRole, isAllowed bool) {
	commandPermission = command.FindString(message.Content)
	actionPermission = action.FindString(message.Content)
	userPermission = user.FindString(message.Content)
	roleIDPermission = role.FindString(message.Content)
	roleNamePermission := roleName.FindString(message.Content)
	isRole = false
	switch permissionValue.FindString(message.Content) {
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
	if roleNamePermission != "" && roleIDPermission == "" {
		name := strings.SplitN(roleNamePermission, "=", 2)[1]
		name = name[1 : len(name)-1]
		roles, err := discordClient.GuildRoles(message.GuildID)
		if err != nil {
			return
		}
		for _, role := range roles {
			if role.Name == name {
				roleIDPermission = role.ID
				message.MentionRoles = append(message.MentionRoles, roleIDPermission)
				return
			}
		}
	}
	return
}

func evaluatePermissions(permissions map[string]bool, command, action string) *bool {
	fmt.Printf("permissions:%v\n", permissions)
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

func listAuthorizationKeys(guildID string) map[string]*dynamodb.KeysAndAttributes {
	keysAndAttributes := map[string]*dynamodb.KeysAndAttributes{
		assets.AuthTableName: &dynamodb.KeysAndAttributes{},
	}
	keys := make([]map[string]*dynamodb.AttributeValue, 0, 10)
	for k, v := range Commands {
		key := make(map[string]*dynamodb.AttributeValue)
		key["guild"] = &dynamodb.AttributeValue{S: aws.String(guildID + "!" + k + "!")}
		keys = append(keys, key)
		for _, action := range v {
			key := make(map[string]*dynamodb.AttributeValue)
			key["guild"] = &dynamodb.AttributeValue{S: aws.String(guildID + "!" + k + "!" + action)}
			keys = append(keys, key)
		}
	}
	keysAndAttributes[assets.AuthTableName].SetKeys(keys[:len(keys)])
	return keysAndAttributes
}

func buildAuthorizationKeys(guildID, userID, command, action string, roleIDList []string) map[string]*dynamodb.KeysAndAttributes {
	keysAndAttributes := map[string]*dynamodb.KeysAndAttributes{
		assets.AuthTableName: &dynamodb.KeysAndAttributes{},
	}

	keys := make([]map[string]*dynamodb.AttributeValue, 0, 10)
	//Construct keys for roles
	for _, role := range roleIDList {
		keys = append(keys, buildAuthorizationKey(guildID, userID, role, command, action, true))
		if action != "" {
			keys = append(keys, buildAuthorizationKey(guildID, userID, role, command, "", true))
		}
	}
	//Add key for userID
	keys = append(keys, buildAuthorizationKey(guildID, userID, "", command, action, false))
	if action != "" {
		keys = append(keys, buildAuthorizationKey(guildID, userID, "", command, "", false))
	}
	keysAndAttributes[assets.AuthTableName].SetKeys(keys[:len(keys)])
	// fmt.Printf("keys and attributes:%v\n\n", keysAndAttributes)
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
	fmt.Printf("key:%v\n", key)
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

func validatePermissionID(userID, roleID string) bool {
	return !(userID == "" && roleID == "")
}
