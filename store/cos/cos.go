package cos

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type Client struct {
	inner     *cos.Client
	bucket    string
	region    string
	secretId  string
	secretKey string
}

func NewClient(bucket, region, secretId, secretKey string) *Client {
	slog.Info("New COS client", "bucket", bucket, "region", region, "secretId", secretId, "secretKey", secretKey)
	bucketUrl, _ := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucket, region))
	c := cos.NewClient(
		&cos.BaseURL{BucketURL: bucketUrl},
		&http.Client{
			Transport: &cos.AuthorizationTransport{
				SecretID:  secretId,
				SecretKey: secretKey,
			},
		},
	)
	return &Client{
		inner:     c,
		bucket:    bucket,
		region:    region,
		secretId:  secretId,
		secretKey: secretKey,
	}
}

func (c *Client) PresignUpload(ctx context.Context,
	objectKey string) (uploadUrl string, accessUrl string,
	err error) {
	presigned, err := c.inner.Object.GetPresignedURL(
		ctx,
		http.MethodPut,
		objectKey,
		c.secretId,
		c.secretKey,
		15*time.Minute,
		nil,
	)
	if err != nil {
		return "", "", err
	}
	accessUrl = fmt.Sprintf(
		"https://%s.cos.%s.myqcloud.com/%s",
		c.bucket, c.region, objectKey,
	)
	return presigned.String(), accessUrl, nil
}
