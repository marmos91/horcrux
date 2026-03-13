package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/marmos91/horcrux/internal/backend"
)

func init() {
	backend.Register("s3", func(opts map[string]string) (backend.Backend, error) {
		bucket := opts["bucket"]
		if bucket == "" {
			return nil, fmt.Errorf("s3 backend requires a bucket (e.g. s3://my-bucket/prefix)")
		}
		return New(context.Background(), bucket, opts["prefix"], opts)
	})
}

// S3 implements backend.Backend using AWS S3.
type S3 struct {
	client *s3.Client
	bucket string
	prefix string
}

// New creates an S3 backend.
func New(ctx context.Context, bucket, prefix string, opts map[string]string) (*S3, error) {
	var cfgOpts []func(*awsconfig.LoadOptions) error

	if region := opts["region"]; region != "" {
		cfgOpts = append(cfgOpts, awsconfig.WithRegion(region))
	}

	if keyID, secret := opts["access-key-id"], opts["secret-access-key"]; keyID != "" && secret != "" {
		cfgOpts = append(cfgOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(keyID, secret, ""),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if endpoint := opts["endpoint"]; endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = &endpoint
		})
	}
	if opts["force-path-style"] == "true" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)

	return &S3{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

func (s *S3) remoteKey(key string) string {
	if s.prefix != "" {
		return s.prefix + "/" + key
	}
	return key
}

func (s *S3) Upload(ctx context.Context, localPath string, remoteKey string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.bucket,
		Key:    strPtr(s.remoteKey(remoteKey)),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("uploading to S3: %w", err)
	}
	return nil
}

func (s *S3) Download(ctx context.Context, remoteKey string, localPath string) error {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    strPtr(s.remoteKey(remoteKey)),
	})
	if err != nil {
		return fmt.Errorf("downloading from S3: %w", err)
	}
	defer func() { _ = result.Body.Close() }()

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, result.Body); err != nil {
		return fmt.Errorf("writing downloaded data: %w", err)
	}
	return out.Close()
}

func (s *S3) List(ctx context.Context, prefix string) ([]backend.RemoteFile, error) {
	fullPrefix := backend.JoinPrefix(s.prefix, prefix)

	var files []backend.RemoteFile
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &fullPrefix,
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing S3 objects: %w", err)
		}
		for _, obj := range page.Contents {
			key := *obj.Key
			if !strings.HasSuffix(key, ".hrcx") {
				continue
			}
			// Strip prefix to return relative keys
			relKey := key
			if s.prefix != "" {
				relKey = strings.TrimPrefix(key, s.prefix+"/")
			}
			files = append(files, backend.RemoteFile{
				Key:  relKey,
				Size: *obj.Size,
			})
		}
	}

	return files, nil
}

func (s *S3) Delete(ctx context.Context, remoteKey string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    strPtr(s.remoteKey(remoteKey)),
	})
	if err != nil {
		return fmt.Errorf("deleting from S3: %w", err)
	}
	return nil
}

func strPtr(s string) *string { return &s }

var _ backend.Backend = (*S3)(nil)
