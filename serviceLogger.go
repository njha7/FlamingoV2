package main

import (
	"log"
	"os"
)

/*
Very opinionated convenience methods for instantiating loggers. Designed for
AWS FARGATE runtime, which by default exports stdout and stderr to Cloudwatch.
Therefore stdout is more desirable than managing logfiles within the container.
Cloudwatch provides UTC timestamps
*/

func BuildServiceLogger(serviceName string) (serviceLogger *log.Logger) {
	serviceLogger = log.New(os.Stdout, serviceName+" - ", 0)
	return
}

func BuildServiceErrorLogger(serviceName string) (serviceErrorLogger *log.Logger) {
	serviceErrorLogger = log.New(os.Stderr, serviceName+" [ERROR] -", 0)
	return
}
