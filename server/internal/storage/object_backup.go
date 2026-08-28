/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

var ErrObjectAlreadyExists = errors.New("backup object already exists")

const (
	defaultMultipartThreshold = int64(5 << 30)
	defaultMultipartPartSize  = int64(64 << 20)
	maxMultipartParts         = int64(10_000)
)

type S3BackupStorage struct {
	client             *s3.Client
	bucket             string
	ossForbidOverwrite bool
	listPageSize       *int32
	multipartThreshold int64
	multipartPartSize  int64
}

func NewS3BackupStorage(ctx context.Context, endpoint, bucket, region, accessKey, secretKey string, pathStyle bool) (*S3BackupStorage, error) {
	return newS3BackupStorage(ctx, endpoint, bucket, region, accessKey, secretKey, pathStyle, false)
}

func NewOSSBackupStorage(ctx context.Context, endpoint, bucket, region, accessKey, secretKey string) (*S3BackupStorage, error) {
	return newS3BackupStorage(ctx, endpoint, bucket, region, accessKey, secretKey, false, true)
}

func newS3BackupStorage(ctx context.Context, endpoint, bucket, region, accessKey, secretKey string, pathStyle, ossForbidOverwrite bool) (*S3BackupStorage, error) {
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.TrimSpace(bucket)
	region = strings.TrimSpace(region)
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	if endpoint == "" || bucket == "" || region == "" || accessKey == "" || secretKey == "" {
		return nil, errors.New("backup object storage requires endpoint, bucket, region, access_key and secret_key")
	}
	loadOptions := []func(*awscfg.LoadOptions) error{awscfg.WithRegion(region), awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""))}
	cfg, err := awscfg.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = pathStyle
	})
	return &S3BackupStorage{client: client, bucket: bucket, ossForbidOverwrite: ossForbidOverwrite, multipartThreshold: defaultMultipartThreshold, multipartPartSize: defaultMultipartPartSize}, nil
}

func (s *S3BackupStorage) Put(key string, reader io.Reader) (int64, string, error) {
	key, err := validateBackupObjectKey(key)
	if err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp("", ".fileshare-backup-upload-*")
	if err != nil {
		return 0, "", err
	}
	path := tmp.Name()
	defer func() { _ = os.Remove(path) }()
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), reader)
	closeErr := tmp.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	if size > s.multipartUploadThreshold() {
		if err := s.putMultipart(key, file, size); err != nil {
			return 0, "", err
		}
		return size, hex.EncodeToString(hash.Sum(nil)), nil
	}
	if err := s.putSingle(key, file, size); err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *S3BackupStorage) putSingle(key string, file *os.File, size int64) error {
	input := &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: file, ContentLength: aws.Int64(size)}
	options := make([]func(*s3.Options), 0, 1)
	if s.ossForbidOverwrite {
		options = append(options, withOSSForbidOverwrite)
	} else {
		input.IfNoneMatch = aws.String("*")
	}
	_, err := s.client.PutObject(context.Background(), input, options...)
	if err != nil {
		if isAlreadyExists(err) {
			return ErrObjectAlreadyExists
		}
		return err
	}
	return nil
}

func (s *S3BackupStorage) putMultipart(key string, file *os.File, size int64) (resultErr error) {
	options := make([]func(*s3.Options), 0, 1)
	if s.ossForbidOverwrite {
		options = append(options, withOSSForbidOverwrite)
	}
	created, err := s.client.CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}, options...)
	if err != nil {
		if isAlreadyExists(err) {
			return ErrObjectAlreadyExists
		}
		return err
	}
	if created.UploadId == nil || *created.UploadId == "" {
		return errors.New("object storage returned an empty multipart upload ID")
	}
	completed := false
	defer func() {
		if !completed {
			_, _ = s.client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: created.UploadId})
		}
	}()

	partSize := s.multipartUploadPartSize(size)
	parts := make([]types.CompletedPart, 0, (size+partSize-1)/partSize)
	for offset, partNumber := int64(0), int32(1); offset < size; offset, partNumber = offset+partSize, partNumber+1 {
		length := min(partSize, size-offset)
		uploaded, err := s.client.UploadPart(context.Background(), &s3.UploadPartInput{
			Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: created.UploadId,
			PartNumber: aws.Int32(partNumber), Body: io.NewSectionReader(file, offset, length), ContentLength: aws.Int64(length),
		})
		if err != nil {
			return err
		}
		if uploaded.ETag == nil || *uploaded.ETag == "" {
			return errors.New("object storage returned an empty multipart ETag")
		}
		parts = append(parts, types.CompletedPart{ETag: uploaded.ETag, PartNumber: aws.Int32(partNumber)})
	}
	input := &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: created.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	}
	if !s.ossForbidOverwrite {
		input.IfNoneMatch = aws.String("*")
	}
	if _, err := s.client.CompleteMultipartUpload(context.Background(), input, options...); err != nil {
		if isAlreadyExists(err) {
			return ErrObjectAlreadyExists
		}
		return err
	}
	completed = true
	return nil
}

func (s *S3BackupStorage) multipartUploadThreshold() int64 {
	if s.multipartThreshold > 0 {
		return s.multipartThreshold
	}
	return defaultMultipartThreshold
}

func (s *S3BackupStorage) multipartUploadPartSize(size int64) int64 {
	partSize := s.multipartPartSize
	if partSize <= 0 {
		partSize = defaultMultipartPartSize
	}
	if minimum := (size + maxMultipartParts - 1) / maxMultipartParts; minimum > partSize {
		partSize = minimum
	}
	return partSize
}

func (s *S3BackupStorage) Get(key string) (io.ReadCloser, error) {
	key, err := validateBackupObjectKey(key)
	if err != nil {
		return nil, err
	}
	output, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		if isArchiveRestoreRequired(err) {
			return nil, ErrArchiveRestoreRequired
		}
		return nil, err
	}
	return output.Body, nil
}

func (s *S3BackupStorage) Delete(key string) error {
	key, err := validateBackupObjectKey(key)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}

func (s *S3BackupStorage) List(prefix string) ([]string, error) {
	prefix = strings.TrimSpace(prefix)
	keys := make([]string, 0)
	var token *string
	for {
		output, err := s.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), Prefix: aws.String(prefix), ContinuationToken: token, MaxKeys: s.listPageSize})
		if err != nil {
			return nil, err
		}
		for _, object := range output.Contents {
			if object.Key != nil {
				keys = append(keys, *object.Key)
			}
		}
		if !aws.ToBool(output.IsTruncated) || output.NextContinuationToken == nil {
			break
		}
		token = output.NextContinuationToken
	}
	return keys, nil
}

func (s *S3BackupStorage) Exists(key string) (bool, error) {
	key, err := validateBackupObjectKey(key)
	if err != nil {
		return false, err
	}
	_, err = s.client.HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, err
}

func validateBackupObjectKey(key string) (string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	if key == "" || strings.HasPrefix(key, "/") || strings.ContainsRune(key, '\x00') {
		return "", ErrInvalidObjectKey
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return "", ErrInvalidObjectKey
		}
	}
	return key, nil
}

func withOSSForbidOverwrite(options *s3.Options) {
	options.APIOptions = append(options.APIOptions, func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("OSSForbidOverwrite", func(ctx context.Context, input middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
			request, ok := input.Request.(*smithyhttp.Request)
			if !ok {
				return middleware.BuildOutput{}, middleware.Metadata{}, errors.New("unexpected OSS request type")
			}
			request.Header.Set("x-oss-forbid-overwrite", "true")
			return next.HandleBuild(ctx, input)
		}), middleware.After)
	})
}

func isAlreadyExists(err error) bool {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 412 {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && strings.EqualFold(apiErr.ErrorCode(), "FileAlreadyExists")
}

func isNotFound(err error) bool {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 404 {
		return true
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), "not found") || strings.Contains(strings.ToLower(fmt.Sprint(err)), "nosuchkey")
}

func isArchiveRestoreRequired(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return strings.EqualFold(apiErr.ErrorCode(), "InvalidObjectState") || strings.EqualFold(apiErr.ErrorCode(), "ObjectNotInActiveTierError")
}
