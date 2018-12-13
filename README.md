# FlamingoV2

## About
This is FlamingoV2. It is the successor to [Flamingo](https://github.com/njha7/Flamingo). It is a [Discord](https://discordapp.com) bot with no intrinsic value. It's purely for the [memes](https://www.youtube.com/watch?v=P9ibDqbfPdY).

## Commands

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
Saves a new a copypasta by alias and posts it. Alias can by any alphanumeric string with no whitespace.

Usage: ```~pasta save $alias $copypasta_text```

#### list
Retrieves a paginated list of all the copypastas saved in the server. Repeated calls to list return the next page.

Usage: ```~pasta list```

## Deployment

### Local
```bash
go get -d -v ./...
go install -v ./...
$GOPATH/bin/FlamingoV2 -local=true -t="DISCORD TOKEN" -ak="AWS ACCESS KEY" -sk="AWS SECRET KEY"
```

### AWS Fargate
Follow the [AWS CD tutorial](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-cd-pipeline.html) and pass the environment variables ```DISCORD_TOKEN```, ```AWS_ACCESS_KEY```, ```AWS_SECRET_KEY``` to the appropriate values.