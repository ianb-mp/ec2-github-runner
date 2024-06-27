package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/sethvargo/go-githubactions"
)

// CreateAndStartEC2Instance creates and starts an EC2 instance with the specified parameters.
// It takes a context, an action, an EC2 client, an IAM client, and various parameters for configuring the instance.
// The function returns the ID of the created instance and an error if any.
func CreateAndStartEC2Instance(ctx context.Context, action *githubactions.Action, ec2Client EC2API, iamClient *iam.Client, ec2AmiId, subnetId, securityGroupId, iamRoleName, instanceType, userData, tagSpecifications string) (string, error) {
	startParams := &ec2.RunInstancesInput{
		ImageId:          aws.String(ec2AmiId),
		InstanceType:     ec2Types.InstanceType(instanceType),
		MaxCount:         aws.Int32(1),
		MinCount:         aws.Int32(1),
		Monitoring:       &ec2Types.RunInstancesMonitoringEnabled{Enabled: aws.Bool(false)},
		SubnetId:         aws.String(subnetId),
		SecurityGroupIds: []string{securityGroupId},
		UserData:         aws.String(base64.StdEncoding.EncodeToString([]byte(userData))),
	}

	if tagSpecifications != "" {
		var tags []ec2Types.TagSpecification
		if err := json.Unmarshal([]byte(tagSpecifications), &tags); err != nil {
			action.Fatalf("Error parsing tag specifications: %v", err)
		}
		startParams.TagSpecifications = tags
	}

	if iamRoleName != "" {
		instanceProfileName, err := GetOrCreateInstanceProfile(ctx, action, iamClient, iamRoleName)
		if err != nil {
			return "", fmt.Errorf("error creating or retrieving instance profile for IAM role name %s: %v", iamRoleName, err)
		}
		startParams.IamInstanceProfile = &ec2Types.IamInstanceProfileSpecification{Name: aws.String(instanceProfileName)}
	}

	runResult, err := ec2Client.RunInstances(ctx, startParams)
	if err != nil {
		return "", fmt.Errorf("error starting EC2 instance: %v", err)
	}
	instanceId := *runResult.Instances[0].InstanceId

	if err := WaitForInstanceRunning(ctx, action, ec2Client, instanceId); err != nil {
		return "", fmt.Errorf("error waiting for instance to be running: %v", err)
	}

	return instanceId, nil
}

// WaitForInstanceRunning waits for the specified EC2 instance to reach the "running" state.
// It continuously checks the instance state using the provided EC2 client until the instance is running.
// The function returns an error if there is an issue describing the instance or if the instance fails to reach the running state within a certain time.
func WaitForInstanceRunning(ctx context.Context, action *githubactions.Action, ec2Client EC2API, instanceId string) error {
	params := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	}

	instanceState := ""
	for instanceState != "running" {
		resp, err := ec2Client.DescribeInstances(ctx, params)
		if err != nil {
			return fmt.Errorf("error describing instance %s: %v", instanceId, err)
		}
		instanceState = string(resp.Reservations[0].Instances[0].State.Name)
		action.Infof("Instance state: %s", instanceState)
		if instanceState != "running" {
			time.Sleep(5 * time.Second)
		}
	}
	action.Infof("Instance %s is now running.", instanceId)
	return nil
}

// GetOrCreateInstanceProfile retrieves an existing instance profile with the specified IAM role name,
// or creates a new instance profile if it doesn't exist. It returns the name of the instance profile
// and any error encountered during the process.
func GetOrCreateInstanceProfile(ctx context.Context, action *githubactions.Action, iamClient IAMAPI, iamRoleName string) (string, error) {
	listProfilesInput := &iam.ListInstanceProfilesInput{}
	profiles, err := iamClient.ListInstanceProfiles(ctx, listProfilesInput)
	if err != nil {
		return "", fmt.Errorf("error listing instance profiles: %v", err)
	}
	for _, profile := range profiles.InstanceProfiles {
		for _, role := range profile.Roles {
			if *role.RoleName == iamRoleName {
				action.Infof("Instance profile for IAM role %s already exists.", iamRoleName)
				return *profile.InstanceProfileName, nil
			}
		}
	}

	createProfileInput := &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String(iamRoleName),
	}
	if _, err := iamClient.CreateInstanceProfile(ctx, createProfileInput); err != nil {
		return "", fmt.Errorf("error creating instance profile: %v", err)
	}
	action.Infof("Created instance profile %s", iamRoleName)

	attachRoleInput := &iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: aws.String(iamRoleName),
		RoleName:            aws.String(iamRoleName),
	}
	if _, err := iamClient.AddRoleToInstanceProfile(ctx, attachRoleInput); err != nil {
		return "", fmt.Errorf("error attaching role to instance profile: %v", err)
	}
	action.Infof("Attached role %s to instance profile %s", iamRoleName, iamRoleName)

	return iamRoleName, nil
}

type CommandId = string

// ExecuteCommandOnEC2Instance executes a command on an EC2 instance using the AWS Systems Manager (SSM) service.
// It returns the command ID and an error (if any).
func ExecuteCommandOnEC2Instance(ctx context.Context, action *githubactions.Action, ssmClient SSMAPI, ec2InstanceId, command string, commandMaxWaitTime int) (CommandId, error) {
	reg, err := IsSSMAgentRegistered(ctx, action, ssmClient, ec2InstanceId, 60, 5)
	if err != nil {
		return "", err
	}
	if !reg {
		return "", fmt.Errorf("SSM agent is not registered or online for instance %s", ec2InstanceId)
	}

	sendCommandInput := &ssm.SendCommandInput{
		InstanceIds:  []string{ec2InstanceId},
		DocumentName: aws.String("AWS-RunShellScript"),
		Parameters: map[string][]string{
			"commands": {command},
		},
	}

	sendCommandResp, err := ssmClient.SendCommand(ctx, sendCommandInput)
	if err != nil {
		return "", fmt.Errorf("error sending command '%s' to EC2 instance %s: %v", command, ec2InstanceId, err)
	}
	commandId := CommandId(*sendCommandResp.Command.CommandId)

	commandInvocationDetails, err := GetCommandInvocationDetails(ctx, action, ssmClient, ec2InstanceId, commandId, commandMaxWaitTime)
	if err != nil {
		return "", fmt.Errorf("error getting command invocation details: %v", err)
	}

	action.Group("Command invocation details")
	action.Infof("ResponseCode: %d", commandInvocationDetails.ResponseCode)
	action.Infof("Status: %s", commandInvocationDetails.Status)
	action.Infof("StdError: %s", *commandInvocationDetails.StandardErrorContent)
	if len(*commandInvocationDetails.StandardOutputContent) < 1000 {
		action.Infof("StdOutput: %s", *commandInvocationDetails.StandardOutputContent)
	} else {
		action.Infof("(enable debug to see full output)")
	}
	action.EndGroup()

	return commandId, nil
}

// GetCommandInvocationDetails retrieves the details of a command invocation from AWS Systems Manager (SSM).
// It returns the *ssm.GetCommandInvocationOutput object containing the command invocation details, or an error if any.
// If the command invocation details are not available within the specified maxWaitTime, it returns a timeout error.
func GetCommandInvocationDetails(ctx context.Context, action *githubactions.Action, ssmClient SSMAPI, ec2InstanceId, commandId CommandId, maxWaitTime int) (*ssm.GetCommandInvocationOutput, error) {
	getCommandParams := &ssm.GetCommandInvocationInput{
		CommandId:  aws.String(commandId),
		InstanceId: aws.String(ec2InstanceId),
	}
	waiter := ssm.NewCommandExecutedWaiter(ssmClient)
	return waiter.WaitForOutput(ctx, getCommandParams, time.Duration(maxWaitTime)*time.Second)
}

// IsSSMAgentRegistered checks if the SSM agent is registered and online for a given EC2 instance.
// The function returns true if the SSM agent is registered and online, false otherwise.
// An error is returned if there was a problem with the SSM client or if the timeout was reached.
func IsSSMAgentRegistered(ctx context.Context, action *githubactions.Action, ssmClient SSMAPI, ec2InstanceId string, timeout, interval int) (bool, error) {
	endTime := time.Now().Add(time.Duration(timeout) * time.Second)

	describeInstanceInfoParams := &ssm.DescribeInstanceInformationInput{
		Filters: []ssmTypes.InstanceInformationStringFilter{
			{
				Key:    aws.String("InstanceIds"),
				Values: []string{ec2InstanceId},
			},
		},
	}

	for time.Now().Before(endTime) {
		resp, err := ssmClient.DescribeInstanceInformation(ctx, describeInstanceInfoParams)
		if err != nil {
			return false, err
		}

		if len(resp.InstanceInformationList) > 0 {
			for _, instanceInfo := range resp.InstanceInformationList {
				if *instanceInfo.InstanceId == ec2InstanceId && instanceInfo.PingStatus == ssmTypes.PingStatusOnline {
					action.Infof("SSM agent is registered and online for instance %s", ec2InstanceId)
					return true, nil
				}
			}
		}
		action.Infof("SSM agent is not registered or not online for instance %s. Waiting...", ec2InstanceId)
		time.Sleep(time.Duration(interval) * time.Second)
	}

	action.Infof("Timeout reached. SSM agent is not registered for instance %s", ec2InstanceId)
	return false, nil
}

// TerminateEC2Instance terminates the specified EC2 instance.
func TerminateEC2Instance(ctx context.Context, action *githubactions.Action, ec2Client EC2API, ec2InstanceId string) error {
	stopParams := &ec2.TerminateInstancesInput{
		InstanceIds: []string{ec2InstanceId},
	}

	_, err := ec2Client.TerminateInstances(ctx, stopParams)
	if err != nil {
		return fmt.Errorf("error stopping EC2 instance %s: %v", ec2InstanceId, err)
	}
	action.Infof("Instance %s is stopping...", ec2InstanceId)
	return nil
}
