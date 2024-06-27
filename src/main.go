package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/sethvargo/go-githubactions"
)

func main() {

	action := githubactions.New()

	err := getInputs(action)
	if err != nil {
		action.Fatalf("%s", err)
	}
}

func getInputs(action *githubactions.Action) error {

	mode := action.GetInput("mode")
	if mode == "" {
		return fmt.Errorf("Required input 'mode' is missing.")
	}
	ec2AmiId := action.GetInput("ec2-image-id")
	subnetId := action.GetInput("subnet-id")
	securityGroupId := action.GetInput("security-group-id")
	iamRoleName := action.GetInput("iam-role-name")
	instanceType := action.GetInput("ec2-instance-type")
	if instanceType == "" {
		instanceType = "t2.micro"
	}
	userData := action.GetInput("user-data")
	tagSpecifications := action.GetInput("tag-specifications")
	ec2InstanceId := action.GetInput("ec2-instance-id")
	command := action.GetInput("command")

	ctx := context.Background()

	commandMaxWaitTime, err := strconv.Atoi(action.GetInput("command-max-wait-secs"))
	if err != nil {
		return err
	}
	if commandMaxWaitTime <= 5 {
		action.Warningf("command-max-wait-secs raised to minimum 6 seconds")
		commandMaxWaitTime = 6
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return err
	}

	ec2Client := ec2.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)
	ssmClient := ssm.NewFromConfig(cfg)

	switch mode {
	case "start":
		if ec2AmiId == "" || subnetId == "" || securityGroupId == "" {

			return fmt.Errorf("Required parameters (ec2AmiId, subnetId, securityGroupId) are missing.")
		}
		instanceId, err := CreateAndStartEC2Instance(ctx, action, ec2Client, iamClient, ec2AmiId, subnetId, securityGroupId, iamRoleName, instanceType, userData, tagSpecifications)
		if err != nil {
			action.Fatalf("Error occurred: %v", err)
		}
		action.Infof("Started EC2 instance with ID: %s", instanceId)
		action.SetOutput("ec2-instance-id", instanceId)

	case "command":
		if ec2InstanceId == "" || command == "" {
			return fmt.Errorf("Required parameters (ec2InstanceId, command) are missing.")
		}
		commandId, err := ExecuteCommandOnEC2Instance(ctx, action, ssmClient, ec2InstanceId, command, commandMaxWaitTime)
		if err != nil {
			return err
		}
		action.Infof("Command '%s' sent to instance %s. Command ID: %s. Command wait time: %d secs", command, ec2InstanceId, commandId, commandMaxWaitTime)
		action.SetOutput("command-id", commandId)

	case "stop":
		if ec2InstanceId == "" {
			return fmt.Errorf("Required parameter (ec2InstanceId) is missing.")
		}
		err := TerminateEC2Instance(ctx, action, ec2Client, ec2InstanceId)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("Unsupported mode: %s. Supported modes are 'start', 'command', and 'stop'.", mode)
	}
	return nil
}
