# FlamingoV2

## About
This is FlamingoV2. It is the successor to [Flamingo](https://github.com/njha7/Flamingo). It is a [Discord](https://discordapp.com) bot with no intrinsic value. It's purely for the [memes](https://www.youtube.com/watch?v=P9ibDqbfPdY).

## Permissions
Flamingo has the ability to restrict access to commands for users. Permissions can be set by role or user directly. 

Permissions are evaluated as follows:

1. User permissions for the command + action
2. User permissions for the command
3. Role permissions for the command + action
4. Role permissions for the command

A command is the archetype of action a user is trying to perform (e.g. pasta) and an action is the exact action (e.g. get).

The first permission rule found using the above order determines a user's permission to execute a given command. Steps 3 and 4 are evaluated for each role in descending guild position. If no rules are found, Flamingo returns the value of the permissive flag for the guild. The permissive flag is set to true when Flamingo joins a guild. A true value treats absent permissons records (as opposed to an explicit allow or deny record) as the equivalent of a present allow. A false value treats absent permissions records as the equivalent of a present deny. The auth command is excluded from this paradigm. Auth requires explicit permission to invoke. By default, only the server owner has this permission. 

## Commands

### auth
Auth commands are used to set permissions.

#### set
Sets the value of a permission rule for a given command and user or role

```Usage: ~auth set command=$command *action=$action ^user=@user ^role=@role permission=$bool```
						
\* - optional argument

^ - XOR

#### delete
Removes a permission rule for a given command and user or role

```Usage: ~auth delete command=$command *action=$action ^user=@user ^role=@role permission=$bool```
						
\* - optional argument

^ - XOR

#### test
Tests a permission rule for a given command and user

```Usage: ~auth test command=$command *action=$action user=@user```
						
\* - optional argument

#### permissive
Sets the value of the permissive flag

```Usage: ~auth permissive permission=$bool```

#### list
Lists the permissions rules for the guild

```Usage: ~auth list```

### strike
Issues a strike to a given user.

Usage: ```~strike @user```

#### get
Retrieves the strike count of a given user.

Usage: ```~strike get @user```

### pasta

#### get
Retrieves a copypasta by alias and posts it. Alias can by any alphanumeric string with no whitespace.

Usage: ```~pasta get $alias```

#### save
Saves a new a copypasta by alias. Alias can by any alphanumeric string with no whitespace.

Usage: ```~pasta save $alias $copypasta_text```

#### edit
Updates an existing copypasta by alias. The copypasta must exist and by authored by the caller for this to succeed.

Usage: ```~pasta save $alias $updated_copypasta_text```

#### list
Retrieves a list of all the copypastas saved in the server and DMs them to the caller.

Usage: ```~pasta list```

### react

#### get
Retrieves a reaction image by alias and posts it. Alias can by any alphanumeric string with no whitespace.

Usage: ```~react get $alias```

#### save
Saves a new a reaction by alias. Reactions are images uploaded to Discord. They are thumbnailed and saved for later reacall. Alias can by any alphanumeric string with no whitespace. Can be used to overwrite an existing reaction.

Usage: ```~react save $alias```

#### delete
Deletes a reaction image and makes it unavailable for use. Alias can by any alphanumeric string with no whitespace.

Usage: ```~react delete $alias```

#### list
Retrieves a list of all the reaction images saved and DMs them to the caller.

Usage: ```~react list```

## Deployment

### Local
```bash
go get -d -v ./...
go install -v ./...
$GOPATH/bin/FlamingoV2 -local=true -t="DISCORD TOKEN" -ak="AWS ACCESS KEY" -sk="AWS SECRET KEY" -r="AWS Region (e.g. us-west-2)"
```

### AWS Fargate
Follow the [AWS CD tutorial](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-cd-pipeline.html) and pass the environment variables ```DISCORD_TOKEN```, ```AWS_ACCESS_KEY```, ```AWS_SECRET_KEY```, ```REGION``` to the appropriate values.