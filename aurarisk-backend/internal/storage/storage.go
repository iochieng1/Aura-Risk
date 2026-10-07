package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"aurarisk-backend/internal/metrics"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("object not found")

type PresignedRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// ObjectStore is the subset of object storage the app needs. Photo bytes
// only ever live here, never in Postgres.
type ObjectStore interface {
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (*PresignedRequest, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Get(ctx context.Context, key string, maxBytes int64) ([]byte, error)
	Put(ctx context.Context, key, contentType string, body []byte) error
	Delete(ctx context.Context, key string) error
}

type S3Config struct {
	Bucket         string
	Region         string
	Endpoint       string
	PublicEndpoint string
	ForcePathStyle bool
}

type S3Store struct {
	bucket    string
	client    *s3.Client
	presigner *s3.PresignClient
}

func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	clientFor := func(endpoint string) *s3.Client {
		return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.UsePathStyle = cfg.ForcePathStyle
			if endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}
		})
	}

	publicEndpoint := cfg.PublicEndpoint
	if publicEndpoint == "" {
		publicEndpoint = cfg.Endpoint
	}

	return &S3Store{
		bucket: cfg.Bucket,
		client: clientFor(cfg.Endpoint),
		// Presigned URLs are opened by phones, so they are signed against the
		// publicly reachable endpoint.
		presigner: s3.NewPresignClient(clientFor(publicEndpoint)),
	}, nil
}

func (s *S3Store) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (*PresignedRequest, error) {
	req, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return nil, err
	}

	headers := map[string]string{}
	for name, values := range req.SignedHeader {
		if name == "Host" || len(values) == 0 {
			continue
		}
		headers[name] = values[0]
	}
	return &PresignedRequest{
		URL:       req.URL,
		Method:    req.Method,
		Headers:   headers,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

func (s *S3Store) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *S3Store) Get(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	start := time.Now()
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	var noKey *types.NoSuchKey
	if errors.As(err, &noKey) {
		// A missing upload is the client's problem, not the store's.
		metrics.ObserveProvider(metrics.ProviderS3, "get", start, nil)
		return nil, ErrNotFound
	}
	metrics.ObserveProvider(metrics.ProviderS3, "get", start, &err)
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	data, err := io.ReadAll(io.LimitReader(out.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("object %s exceeds %d bytes", key, maxBytes)
	}
	return data, nil
}

func (s *S3Store) Put(ctx context.Context, key, contentType string, body []byte) (err error) {
	defer metrics.ObserveProvider(metrics.ProviderS3, "put", time.Now(), &err)
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(body))),
		Body:          bytes.NewReader(body),
		CacheControl:  aws.String("private, max-age=86400, immutable"),
	})
	return err
}

func (s *S3Store) Delete(ctx context.Context, key string) (err error) {
	defer metrics.ObserveProvider(metrics.ProviderS3, "delete", time.Now(), &err)
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}
