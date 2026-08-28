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
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"file-share-manager/server/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

func TestValidateBackupObjectKey(t *testing.T) {
	for _, key := range []string{"", "/absolute", "../escape", "folder/../escape", "folder/..", "folder/./object", "folder//object", ".."} {
		if _, err := validateBackupObjectKey(key); !errors.Is(err, ErrInvalidObjectKey) {
			t.Fatalf("validateBackupObjectKey(%q) = %v", key, err)
		}
	}
	if got, err := validateBackupObjectKey("folder\\object"); err != nil || got != "folder/object" {
		t.Fatalf("normalized key = %q, err = %v", got, err)
	}
}

func TestNewConfiguredBackupStorageLocal(t *testing.T) {
	store, err := NewConfiguredBackupStorage(t.Context(), config.BackupConfig{Type: "local", LocalPath: t.TempDir()})
	if err != nil || store == nil {
		t.Fatalf("local factory = %#v, %v", store, err)
	}
}

func TestNewConfiguredBackupStorageRequiresObjectCredentials(t *testing.T) {
	_, err := NewConfiguredBackupStorage(t.Context(), config.BackupConfig{Type: "s3", Endpoint: "https://s3.example", Bucket: "fileshare", Region: "us-east-1"})
	if err == nil {
		t.Fatal("expected object storage credentials to be required")
	}
}

func TestNewConfiguredBackupStorageAddressingModes(t *testing.T) {
	base := config.BackupConfig{Endpoint: "https://storage.example", Bucket: "fileshare", Region: "us-east-1", AccessKey: "access", SecretKey: "secret"}
	for _, test := range []struct {
		name               string
		storageType        string
		ossForbidOverwrite bool
	}{
		{name: "s3", storageType: "s3"},
		{name: "minio", storageType: "minio"},
		{name: "oss", storageType: "oss", ossForbidOverwrite: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			cfg.Type = test.storageType
			configured, err := NewConfiguredBackupStorage(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			store, ok := configured.(*S3BackupStorage)
			if !ok || store.ossForbidOverwrite != test.ossForbidOverwrite {
				t.Fatalf("store = %#v", configured)
			}
		})
	}
}

func TestS3BackupStorageHTTPContract(t *testing.T) {
	objects := map[string][]byte{}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		const bucketPath = "/fileshare-test"
		if request.URL.Path == bucketPath && request.Method == http.MethodGet && request.URL.Query().Get("list-type") == "2" {
			if request.URL.Query().Get("continuation-token") == "page-2" {
				return httpTestResponse(request, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>fileshare-test</Name><Prefix>integration/</Prefix><KeyCount>1</KeyCount><MaxKeys>2</MaxKeys><IsTruncated>false</IsTruncated><Contents><Key>integration/c.txt</Key><Size>3</Size></Contents></ListBucketResult>`), nil
			}
			return httpTestResponse(request, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>fileshare-test</Name><Prefix>integration/</Prefix><KeyCount>2</KeyCount><MaxKeys>2</MaxKeys><IsTruncated>true</IsTruncated><NextContinuationToken>page-2</NextContinuationToken><Contents><Key>integration/a.txt</Key><Size>3</Size></Contents><Contents><Key>integration/b.txt</Key><Size>3</Size></Contents></ListBucketResult>`), nil
		}
		key := strings.TrimPrefix(request.URL.Path, bucketPath+"/")
		if key == request.URL.Path || key == "" {
			return httpTestResponse(request, http.StatusNotFound, ""), nil
		}
		switch request.Method {
		case http.MethodPut:
			if request.Header.Get("If-None-Match") != "*" {
				return httpTestResponse(request, http.StatusBadRequest, "missing conditional write"), nil
			}
			if _, exists := objects[key]; exists {
				return httpTestResponse(request, http.StatusPreconditionFailed, ""), nil
			}
			payload, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			objects[key] = payload
			return httpTestResponse(request, http.StatusOK, ""), nil
		case http.MethodGet:
			payload, exists := objects[key]
			if !exists {
				return httpTestResponse(request, http.StatusNotFound, ""), nil
			}
			return httpTestResponse(request, http.StatusOK, string(payload)), nil
		case http.MethodHead:
			payload, exists := objects[key]
			if !exists {
				return httpTestResponse(request, http.StatusNotFound, ""), nil
			}
			response := httpTestResponse(request, http.StatusOK, "")
			response.Header.Set("Content-Length", fmt.Sprint(len(payload)))
			return response, nil
		default:
			return httpTestResponse(request, http.StatusMethodNotAllowed, ""), nil
		}
	})

	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
		HTTPClient:  &http.Client{Transport: transport},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://storage.example")
		options.UsePathStyle = true
	})
	store := &S3BackupStorage{client: client, bucket: "fileshare-test"}
	size, digest, err := store.Put("integration/object.txt", strings.NewReader("payload"))
	if err != nil || size != 7 || len(digest) != 64 {
		t.Fatalf("Put = %d, %q, %v", size, digest, err)
	}
	if _, _, err := store.Put("integration/object.txt", strings.NewReader("overwrite")); !errors.Is(err, ErrObjectAlreadyExists) {
		t.Fatalf("second Put = %v", err)
	}
	exists, err := store.Exists("integration/object.txt")
	if err != nil || !exists {
		t.Fatalf("Exists(existing) = %v, %v", exists, err)
	}
	exists, err = store.Exists("integration/missing.txt")
	if err != nil || exists {
		t.Fatalf("Exists(missing) = %v, %v", exists, err)
	}
	reader, err := store.Get("integration/object.txt")
	if err != nil {
		t.Fatal(err)
	}
	payload, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || string(payload) != "payload" {
		t.Fatalf("Get = %q, %v, %v", payload, readErr, closeErr)
	}
	pageSize := int32(2)
	store.listPageSize = &pageSize
	listed, err := store.List("integration/")
	if err != nil || !slices.Equal(listed, []string{"integration/a.txt", "integration/b.txt", "integration/c.txt"}) {
		t.Fatalf("List = %#v, %v", listed, err)
	}
}

func TestOSSBackupStorageHTTPContract(t *testing.T) {
	requestCount := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.URL.Host != "fileshare-test.storage.example" || request.URL.Path != "/integration/object.txt" {
			t.Fatalf("OSS request URL = %s", request.URL.String())
		}
		if request.Header.Get("x-oss-forbid-overwrite") != "true" || request.Header.Get("If-None-Match") != "" {
			t.Fatalf("OSS conditional headers = %#v", request.Header)
		}
		if requestCount == 1 {
			return httpTestResponse(request, http.StatusOK, ""), nil
		}
		return httpTestResponse(request, http.StatusConflict, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>FileAlreadyExists</Code><Message>Object already exists</Message></Error>`), nil
	})
	client := s3.NewFromConfig(aws.Config{
		Region:      "oss-cn-hangzhou",
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
		HTTPClient:  &http.Client{Transport: transport},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://storage.example")
		options.UsePathStyle = false
	})
	store := &S3BackupStorage{client: client, bucket: "fileshare-test", ossForbidOverwrite: true}
	if _, _, err := store.Put("integration/object.txt", strings.NewReader("payload")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Put("integration/object.txt", strings.NewReader("overwrite")); !errors.Is(err, ErrObjectAlreadyExists) {
		t.Fatalf("second OSS Put = %v", err)
	}
}

func TestS3BackupStorageMultipartContract(t *testing.T) {
	var uploadedParts []string
	completed := false
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/fileshare-test/integration/large.bin" {
			t.Fatalf("multipart URL = %s", request.URL.String())
		}
		query := request.URL.Query()
		switch {
		case request.Method == http.MethodPost && query.Has("uploads"):
			return httpTestResponse(request, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><InitiateMultipartUploadResult><Bucket>fileshare-test</Bucket><Key>integration/large.bin</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`), nil
		case request.Method == http.MethodPut && query.Get("uploadId") == "upload-1":
			payload, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			uploadedParts = append(uploadedParts, string(payload))
			response := httpTestResponse(request, http.StatusOK, "")
			response.Header.Set("ETag", fmt.Sprintf(`"part-%s"`, query.Get("partNumber")))
			return response, nil
		case request.Method == http.MethodPost && query.Get("uploadId") == "upload-1":
			if request.Header.Get("If-None-Match") != "*" {
				t.Fatalf("complete If-None-Match = %q", request.Header.Get("If-None-Match"))
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			if !strings.Contains(string(body), "part-1") || !strings.Contains(string(body), "part-2") || !strings.Contains(string(body), "part-3") {
				t.Fatalf("complete body = %s", body)
			}
			completed = true
			return httpTestResponse(request, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUploadResult><Bucket>fileshare-test</Bucket><Key>integration/large.bin</Key><ETag>complete</ETag></CompleteMultipartUploadResult>`), nil
		case request.Method == http.MethodDelete:
			t.Fatal("completed multipart upload must not be aborted")
		}
		return httpTestResponse(request, http.StatusBadRequest, ""), nil
	})
	client := s3.NewFromConfig(aws.Config{
		Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
		HTTPClient: &http.Client{Transport: transport},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://storage.example")
		options.UsePathStyle = true
	})
	store := &S3BackupStorage{client: client, bucket: "fileshare-test", multipartThreshold: 4, multipartPartSize: 4}
	size, digest, err := store.Put("integration/large.bin", strings.NewReader("abcdefghij"))
	if err != nil || size != 10 || len(digest) != 64 || !completed || !slices.Equal(uploadedParts, []string{"abcd", "efgh", "ij"}) {
		t.Fatalf("multipart Put = %d, %q, %v, completed=%v, parts=%#v", size, digest, err, completed, uploadedParts)
	}
}

func TestS3BackupStorageMultipartFailureAborts(t *testing.T) {
	aborted := false
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		query := request.URL.Query()
		switch {
		case request.Method == http.MethodPost && query.Has("uploads"):
			return httpTestResponse(request, http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?><InitiateMultipartUploadResult><Bucket>fileshare-test</Bucket><Key>integration/large.bin</Key><UploadId>upload-2</UploadId></InitiateMultipartUploadResult>`), nil
		case request.Method == http.MethodPut:
			return httpTestResponse(request, http.StatusInternalServerError, `<Error><Code>InternalError</Code></Error>`), nil
		case request.Method == http.MethodDelete && query.Get("uploadId") == "upload-2":
			aborted = true
			return httpTestResponse(request, http.StatusNoContent, ""), nil
		}
		return httpTestResponse(request, http.StatusBadRequest, ""), nil
	})
	client := s3.NewFromConfig(aws.Config{
		Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
		HTTPClient: &http.Client{Transport: transport},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://storage.example")
		options.UsePathStyle = true
	})
	store := &S3BackupStorage{client: client, bucket: "fileshare-test", multipartThreshold: 4, multipartPartSize: 4}
	if _, _, err := store.Put("integration/large.bin", strings.NewReader("abcdefghij")); err == nil || !aborted {
		t.Fatalf("multipart failure = %v, aborted = %v", err, aborted)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func httpTestResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"application/xml"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestS3BackupStorageIntegration(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("FILESHARE_TEST_S3_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("FILESHARE_TEST_S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("FILESHARE_TEST_S3_SECRET_KEY"))
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Skip("set FILESHARE_TEST_S3_ENDPOINT, FILESHARE_TEST_S3_ACCESS_KEY and FILESHARE_TEST_S3_SECRET_KEY")
	}
	region := strings.TrimSpace(os.Getenv("FILESHARE_TEST_S3_REGION"))
	if region == "" {
		region = "us-east-1"
	}
	bucket := "fileshare-test-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	store, err := NewS3BackupStorage(t.Context(), endpoint, bucket, region, accessKey, secretKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.client.CreateBucket(t.Context(), &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	keys := []string{"integration/a.txt", "integration/b.txt", "integration/c.txt"}
	t.Cleanup(func() {
		for _, key := range keys {
			_, _ = store.client.DeleteObject(t.Context(), &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		}
		_, _ = store.client.DeleteBucket(t.Context(), &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	for _, key := range keys {
		payload := []byte("payload:" + key)
		size, digest, err := store.Put(key, bytes.NewReader(payload))
		if err != nil || size != int64(len(payload)) || len(digest) != 64 {
			t.Fatalf("Put(%q) = %d, %q, %v", key, size, digest, err)
		}
	}
	if _, _, err := store.Put(keys[0], strings.NewReader("overwrite")); !errors.Is(err, ErrObjectAlreadyExists) {
		t.Fatalf("second Put = %v", err)
	}

	exists, err := store.Exists(keys[0])
	if err != nil || !exists {
		t.Fatalf("Exists(existing) = %v, %v", exists, err)
	}
	exists, err = store.Exists("integration/missing.txt")
	if err != nil || exists {
		t.Fatalf("Exists(missing) = %v, %v", exists, err)
	}

	reader, err := store.Get(keys[1])
	if err != nil {
		t.Fatal(err)
	}
	payload, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || string(payload) != "payload:"+keys[1] {
		t.Fatalf("Get = %q, %v, %v", payload, readErr, closeErr)
	}

	pageSize := int32(2)
	store.listPageSize = &pageSize
	listed, err := store.List("integration/")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(listed)
	if !slices.Equal(listed, keys) {
		t.Fatalf("List = %#v", listed)
	}
}
