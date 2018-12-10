package main

import (
	"os"
	"log"
)

/*
Very opinionated convenience methods for instantiating loggers. Designed for 
AWS FARGATE runtime, which by default exports stdout and stderr to Cloudwatch.
Therefore stdout is more desirable than managing logfiles within the container.
Local time is just a convenient preference.
*/

func BuildServiceLogger(serviceName string) (serviceLogger *log.Logger) {
	serviceLogger = log.New(os.Stdout, serviceName + "-", log.Ltime)
	return
}

func BuildServiceErrorLogger(serviceName string) (serviceErrorLogger *log.Logger) {
	serviceErrorLogger = log.New(os.Stderr, serviceName + "-", log.Ltime)
	return
}