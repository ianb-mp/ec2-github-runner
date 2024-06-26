package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/sethvargo/go-githubactions"
)

const testEC2ClientId = "i-1234567890abcdef0"

// Mock implementations

type MockEC2Client struct{}

func (m *MockEC2Client) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2Types.Reservation{
			{
				Instances: []ec2Types.Instance{
					{
						InstanceId: aws.String(testEC2ClientId),
						State: &ec2Types.InstanceState{
							Name: ec2Types.InstanceStateNameRunning,
						},
					},
				},
			},
		},
	}, nil
}

func (m *MockEC2Client) RunInstances(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {

	return &ec2.RunInstancesOutput{
		Instances: []ec2Types.Instance{
			{
				InstanceId: aws.String(testEC2ClientId),
			},
		},
	}, nil
}

func (m *MockEC2Client) TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {

	return &ec2.TerminateInstancesOutput{
		TerminatingInstances: []ec2Types.InstanceStateChange{
			{
				InstanceId: aws.String(testEC2ClientId),
				CurrentState: &ec2Types.InstanceState{
					Name: ec2Types.InstanceStateNameTerminated,
				},
			},
		},
	}, nil
}

type MockSSMClient struct{}

func (m *MockSSMClient) DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {

	return &ssm.DescribeInstanceInformationOutput{
		InstanceInformationList: []ssmTypes.InstanceInformation{
			{
				InstanceId: aws.String(testEC2ClientId),
				PingStatus: ssmTypes.PingStatusOnline,
			},
		},
	}, nil
}

func (m *MockSSMClient) SendCommand(ctx context.Context, params *ssm.SendCommandInput, optFns ...func(*ssm.Options)) (*ssm.SendCommandOutput, error) {

	return &ssm.SendCommandOutput{
		Command: &ssmTypes.Command{
			CommandId: aws.String("command-id-123"),
		},
	}, nil
}

func (m *MockSSMClient) GetCommandInvocation(ctx context.Context, params *ssm.GetCommandInvocationInput, optFns ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error) {

	return &ssm.GetCommandInvocationOutput{
		CommandId:             aws.String("command-id-123"),
		InstanceId:            aws.String(testEC2ClientId),
		Status:                ssmTypes.CommandInvocationStatusSuccess,
		ResponseCode:          200,
		StandardOutputContent: aws.String("Hello World!"),
		StandardErrorContent:  aws.String(""),
	}, nil
}

type MockIAMClient struct{}

func (m *MockIAMClient) ListInstanceProfiles(ctx context.Context, params *iam.ListInstanceProfilesInput, optFns ...func(*iam.Options)) (*iam.ListInstanceProfilesOutput, error) {
	return &iam.ListInstanceProfilesOutput{
		InstanceProfiles: []iamTypes.InstanceProfile{
			{
				InstanceProfileName: aws.String("test-role"),
				Roles: []iamTypes.Role{
					{
						RoleName: aws.String("test-role"),
					},
				},
			},
		},
	}, nil
}

func (m *MockIAMClient) CreateInstanceProfile(ctx context.Context, params *iam.CreateInstanceProfileInput, optFns ...func(*iam.Options)) (*iam.CreateInstanceProfileOutput, error) {
	return &iam.CreateInstanceProfileOutput{}, nil
}

func (m *MockIAMClient) AddRoleToInstanceProfile(ctx context.Context, params *iam.AddRoleToInstanceProfileInput, optFns ...func(*iam.Options)) (*iam.AddRoleToInstanceProfileOutput, error) {
	return &iam.AddRoleToInstanceProfileOutput{}, nil
}

// Unit tests

func TestWaitForInstanceRunning(t *testing.T) {
	action := githubactions.New()
	mockEC2 := &MockEC2Client{}
	instanceId := testEC2ClientId

	ctx := context.Background()

	err := WaitForInstanceRunning(ctx, action, mockEC2, instanceId)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGetOrCreateInstanceProfile(t *testing.T) {
	action := githubactions.New()
	mockIAM := &MockIAMClient{}
	iamRoleName := "test-role"

	ctx := context.Background()

	profileName, err := GetOrCreateInstanceProfile(ctx, action, mockIAM, iamRoleName)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if profileName != iamRoleName {
		t.Fatalf("expected profile name %s, got %s", iamRoleName, profileName)
	}
}

func TestIsSSMAgentRegistered(t *testing.T) {
	action := githubactions.New()
	mockSSM := &MockSSMClient{}
	instanceId := testEC2ClientId + "xx"

	ctx := context.Background()

	isRegistered, err := IsSSMAgentRegistered(ctx, action, mockSSM, instanceId, 0, 0)
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}

	if isRegistered {
		t.Fatalf("expected agent to NOT be registered, but it was!?")
	}
}

func TestExecuteCommandOnEC2Instance(t *testing.T) {
	action := githubactions.New()
	mockSSM := &MockSSMClient{}
	instanceId := testEC2ClientId
	command := "echo 'Hello, World!'"
	commandMaxWaitTime := 60

	ctx := context.Background()

	commandId, err := ExecuteCommandOnEC2Instance(ctx, action, mockSSM, instanceId, command, commandMaxWaitTime)
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}
	cid, err := GetCommandInvocationDetails(ctx, action, mockSSM, instanceId, commandId, commandMaxWaitTime)
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}

	if cid.Status != ssmTypes.CommandInvocationStatusSuccess {
		t.Fatalf("expected command status to be Success, got %s", cid.Status)
	}
}

func TestTerminateEC2Instance(t *testing.T) {
	action := githubactions.New()
	mockEC2 := &MockEC2Client{}
	instanceId := testEC2ClientId

	ctx := context.Background()

	err := TerminateEC2Instance(ctx, action, mockEC2, instanceId)
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}
}
