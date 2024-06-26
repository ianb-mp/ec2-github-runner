# AWS EC2 Manager GitHub Action

This GitHub Action allows you to manage AWS EC2 instances. You can use it to start, execute commands on, and stop EC2 instances.

[Inputs](##inputs) | [Usage](#usage) | [Example Workflow](#example-workflow) | [Development](#development) | [Credit](#credit)

---

## Inputs

| Parameter               | Description                                            | Required                  | Default    |
|-------------------------|--------------------------------------------------------|---------------------------|------------|
| `mode`                  | The operation mode: `start`, `command`, `stop`         | true                      | N/A        |
| `ec2-image-id`          | The AMI ID for the instance                            | true (for `start` mode)   | N/A        |
| `subnet-id`             | The Subnet ID for the instance                         | true (for `start` mode)   | N/A        |
| `security-group-id`     | The Security Group ID for the instance                 | true (for `start` mode)   | N/A        |
| `iam-role-name`         | IAM role name for the instance profile                 | false                     | N/A        |
| `ec2-instance-type`     | The instance type (e.g., `t3.micro`)                   | false                     | `t3.micro` |
| `user-data`             | The User Data script to configure the instance         | false                     | N/A        |
| `tag-specifications`    | The Tag Specifications for the instance in JSON format | false                     | N/A        |
| `ec2-instance-id`       | The EC2 Instance ID                                    | true                      | N/A        |
| `command`               | The command to execute on the instance                 | true (for `command` mode) | N/A        |
| `command-max-wait-secs` | The command timeout value                              | false                     | 300        |

## Outputs

| Output            | Description                                                |
|-------------------|------------------------------------------------------------|
| `ec2-instance-id` | The ID of the launched EC2 instance (only in `start` mode) |
| `command-id`      | The ID of the command invocation (only in `command` mode)  |

## Usage

### Prerequisites

Ensure that the necessary AWS credentials and region are set up in the environment where this action runs. You can set these as environment variables in your GitHub Actions workflow:

```yaml
env:
  AWS_REGION: your-aws-region
  AWS_ACCESS_KEY_ID: your-access-key-id
  AWS_SECRET_ACCESS_KEY: your-secret-access-key
```

Or use https://github.com/aws-actions/configure-aws-credentials action.

## Example Workflow

```yaml
name: Manage EC2 Instance

on: [push]

jobs:
  manage-ec2:
    runs-on: ubuntu-latest
    steps:
    - name: Checkout code
      uses: actions/checkout@v2

    - name: Start EC2 instance
      uses: your-github-username/your-repo-name@main
      id: start_ec2
      with:
        mode: start
        ec2-image-id: ami-0abcdef1234567890
        subnet-id: subnet-12345678
        security-group-id: sg-12345678
        iam-role-name: my-iam-role-name
        ec2-instance-type: t3.micro
        user-data: |
          #!/bin/bash
          echo "Hello, World!" > /var/www/html/index.html
        tag-specifications: '[{"ResourceType":"instance","Tags":[{"Key":"Name","Value":"MyInstance"}]}]'

    - name: Execute command on EC2 instance
      uses: your-github-username/your-repo-name@main
      with:
        mode: command
        ec2-instance-id: ${{ steps.start_ec2.outputs.ec2-instance-id }}
        command: echo "This is a command executed via SSM"

    - name: Stop EC2 instance
      uses: your-github-username/your-repo-name@main
      with:
        mode: stop
        ec2-instance-id: ${{ steps.start_ec2.outputs.ec2-instance-id }}
```

## IAM Permissions

To use this GitHub Action, the following IAM permissions are required for each mode:

| Mode      | IAM Permissions                                                                                   |
|-----------|---------------------------------------------------------------------------------------------------|
| `start`   | `ec2:RunInstances`, `ec2:DescribeInstances`, `iam:ListInstanceProfiles`, `iam:CreateInstanceProfile`, `iam:AddRoleToInstanceProfile`, `iam:PassRole` |
| `command` | `ssm:SendCommand`, `ssm:ListCommandInvocations`, `ssm:DescribeInstanceInformation`                |
| `stop`    | `ec2:TerminateInstances`  


## Development

Run npm commands with Podman/Docker like this:
```
$ podman run --rm -t \
  -v <path to repo>:/repo \
  -w /repo \
  docker.io/library/node:current-slim \
  npm run lint
```

## Credit

Based on https://github.com/machulav/ec2-github-runner