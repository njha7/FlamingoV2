package assets

// A collection of project-wide constants all in one place
const (
	// AvatarURL is a link to the profile picture used in embed on behalf of Flamingo
	AvatarURL = "https://s3.amazonaws.com/flamingo-bot/pfp"
	// BucketName is the S3 bucket where Flamingo stores assets
	BucketName = "flamingo-bot"
	// StrikeTableName is the name of the table where strikes are persisted
	StrikeTableName = "FlamingoStrikes"
	// PastaTableName is the name of the table where pastas are persisted
	PastaTableName = "FlamingoPasta"
	// AuthTableName is the name of the table where permissions are persisted
	AuthTableName = "FlamingoAuth"
	// CloudWatchNameSpace is the root of the namespace of all metrics emitted by Flamingo
	CloudWatchNamespace = "Flamingo/"
)
