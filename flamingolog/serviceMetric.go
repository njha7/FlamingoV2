package flamingolog

import (
	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

// FlamingoMetricsClient is a singleton responsible for publishing service metrics
type FlamingoMetricsClient struct {
	CloudWatchAgent *cloudwatch.CloudWatch
	Local           bool
}
