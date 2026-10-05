package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type awsSecretsManager struct {
	client *secretsmanager.Client
	region string
}

// NewAWSSecretsManager builds a fetcher from the standard AWS credential chain.
func NewAWSSecretsManager(ctx context.Context) (SecretFetcher, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &awsSecretsManager{client: secretsmanager.NewFromConfig(cfg), region: cfg.Region}, nil
}

func (a *awsSecretsManager) GetSecret(ctx context.Context, id string) (string, error) {
	// A full ARN carries its own region, which also covers an unset AWS_REGION.
	region := a.region
	if parts := strings.Split(id, ":"); len(parts) >= 7 && parts[0] == "arn" {
		region = parts[3]
	}
	if region == "" {
		return "", errors.New("AWS_REGION is not set and the secret id is not an ARN")
	}

	out, err := a.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(id)},
		func(o *secretsmanager.Options) { o.Region = region })
	if err != nil {
		return "", err
	}
	switch {
	case out.SecretString != nil:
		return *out.SecretString, nil
	case out.SecretBinary != nil:
		return string(out.SecretBinary), nil
	default:
		return "", fmt.Errorf("secret has no value")
	}
}
