package object

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	bucket        string
	publicBaseURL string
	region        string
	s3            *s3.Client
	uploader      *manager.Uploader
}

type Config struct {
	Region          string
	Bucket          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	PublicBaseURL   string
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		loadOptions = append(loadOptions,
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
		)
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	return &Client{
		bucket:        cfg.Bucket,
		region:        cfg.Region,
		publicBaseURL: strings.TrimSuffix(cfg.PublicBaseURL, "/"),
		s3:            client,
		uploader:      manager.NewUploader(client),
	}, nil
}

func (c *Client) UploadFile(ctx context.Context, objectKey, absolutePath string) (string, error) {
	file, err := os.Open(absolutePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = c.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(objectKey),
		Body:   file,
	})
	if err != nil {
		return "", err
	}
	return c.ObjectURL(objectKey), nil
}

func (c *Client) ObjectURL(objectKey string) string {
	if c.publicBaseURL != "" {
		return fmt.Sprintf("%s/%s", c.publicBaseURL, url.PathEscape(objectKey))
	}
	return fmt.Sprintf("s3://%s/%s", c.bucket, filepath.ToSlash(objectKey))
}

func (c *Client) OpenRead(ctx context.Context, objectKey string) (io.ReadCloser, string, int64, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, "", 0, err
	}
	contentType := "application/octet-stream"
	if out.ContentType != nil && *out.ContentType != "" {
		contentType = *out.ContentType
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, contentType, size, nil
}
