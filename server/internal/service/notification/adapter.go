/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxNotificationResponseBytes = 64 << 10

var notificationHTTPClient = &http.Client{
	Timeout:       10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	Transport:     &http.Transport{MaxIdleConns: 20, MaxIdleConnsPerHost: 5, IdleConnTimeout: time.Minute},
}

type Message struct {
	Title   string
	Content string
}

func ValidateSettings(channelType string, settings *ChannelSettings) error {
	if settings == nil {
		return errors.New("通知渠道配置不能为空")
	}
	channelType = strings.ToLower(strings.TrimSpace(channelType))
	switch channelType {
	case "dingtalk", "feishu", "wecom":
		settings.WebhookURL = strings.TrimSpace(settings.WebhookURL)
		settings.Secret = strings.TrimSpace(settings.Secret)
		parsed, err := url.Parse(settings.WebhookURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			return errors.New("Webhook 必须是绝对 HTTPS 地址")
		}
		allowedHosts := map[string]map[string]bool{
			"dingtalk": {"oapi.dingtalk.com": true},
			"feishu":   {"open.feishu.cn": true, "open.larksuite.com": true},
			"wecom":    {"qyapi.weixin.qq.com": true},
		}
		if parsed.Port() != "" && parsed.Port() != "443" || !allowedHosts[channelType][strings.ToLower(parsed.Hostname())] {
			return errors.New("Webhook 域名与渠道类型不匹配")
		}
		if len(settings.WebhookURL) > 2048 || len(settings.Secret) > 512 {
			return errors.New("Webhook 或签名密钥长度超限")
		}
	case "smtp":
		settings.SMTPHost = strings.TrimSpace(settings.SMTPHost)
		settings.SMTPUsername = strings.TrimSpace(settings.SMTPUsername)
		settings.SMTPFrom = strings.TrimSpace(settings.SMTPFrom)
		settings.SMTPEncryption = strings.ToLower(strings.TrimSpace(settings.SMTPEncryption))
		if settings.SMTPHost == "" || strings.ContainsAny(settings.SMTPHost, "\r\n\t /\\") || len(settings.SMTPHost) > 255 {
			return errors.New("SMTP 主机无效")
		}
		if settings.SMTPPort < 1 || settings.SMTPPort > 65535 {
			return errors.New("SMTP 端口无效")
		}
		if settings.SMTPEncryption != "tls" && settings.SMTPEncryption != "starttls" {
			return errors.New("SMTP 必须使用 TLS 或 STARTTLS")
		}
		if settings.SMTPUsername != "" && settings.SMTPPassword == "" {
			return errors.New("SMTP 用户名已配置时密码不能为空")
		}
		from, err := mail.ParseAddress(settings.SMTPFrom)
		if err != nil || strings.ContainsAny(settings.SMTPFrom, "\r\n") {
			return errors.New("发件人地址无效")
		}
		settings.SMTPFrom = from.Address
		if len(settings.SMTPRecipients) == 0 || len(settings.SMTPRecipients) > 20 {
			return errors.New("收件人数量必须在 1 到 20 之间")
		}
		recipients := make([]string, 0, len(settings.SMTPRecipients))
		seen := map[string]bool{}
		for _, raw := range settings.SMTPRecipients {
			address, err := mail.ParseAddress(strings.TrimSpace(raw))
			if err != nil || strings.ContainsAny(raw, "\r\n") {
				return errors.New("收件人地址无效")
			}
			key := strings.ToLower(address.Address)
			if !seen[key] {
				seen[key] = true
				recipients = append(recipients, address.Address)
			}
		}
		settings.SMTPRecipients = recipients
	default:
		return errors.New("不支持的通知渠道类型")
	}
	return nil
}

func Deliver(ctx context.Context, channelType string, settings ChannelSettings, message Message) error {
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case "dingtalk":
		endpoint := settings.WebhookURL
		if settings.Secret != "" {
			endpoint = signDingTalkWebhook(endpoint, settings.Secret, time.Now())
		}
		return postWebhook(ctx, endpoint, map[string]any{"msgtype": "text", "text": map[string]string{"content": message.Title + "\n" + message.Content}})
	case "feishu":
		payload := map[string]any{"msg_type": "text", "content": map[string]string{"text": message.Title + "\n" + message.Content}}
		if settings.Secret != "" {
			addFeishuSignature(payload, settings.Secret, time.Now())
		}
		return postWebhook(ctx, settings.WebhookURL, payload)
	case "wecom":
		return postWebhook(ctx, settings.WebhookURL, map[string]any{"msgtype": "text", "text": map[string]string{"content": message.Title + "\n" + message.Content}})
	case "smtp":
		return sendSMTP(ctx, settings, message)
	default:
		return errors.New("unsupported notification channel")
	}
}

func signDingTalkWebhook(endpoint, secret string, now time.Time) string {
	timestamp := strconv.FormatInt(now.UnixMilli(), 10)
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(timestamp + "\n" + secret))
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	return endpoint + separator + "timestamp=" + timestamp + "&sign=" + url.QueryEscape(base64.StdEncoding.EncodeToString(h.Sum(nil)))
}

func addFeishuSignature(payload map[string]any, secret string, now time.Time) {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	h := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	_, _ = h.Write(nil)
	payload["timestamp"] = timestamp
	payload["sign"] = base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func postWebhook(ctx context.Context, endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := notificationHTTPClient.Do(req)
	if err != nil {
		// net/http errors include the full request URL, which may contain a webhook token.
		return errors.New("Webhook 请求失败，请检查网络和渠道配置")
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxNotificationResponseBytes))
	if readErr != nil {
		return errors.New("无法读取 Webhook 响应")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook 返回异常状态码 HTTP %d", resp.StatusCode)
	}
	var result map[string]any
	if len(responseBody) > 0 && json.Unmarshal(responseBody, &result) == nil {
		for _, field := range []string{"errcode", "StatusCode", "code"} {
			if value, ok := result[field]; ok && fmt.Sprint(value) != "0" && fmt.Sprint(value) != "0.0" {
				return fmt.Errorf("Webhook 拒绝请求（%s=%v）", field, value)
			}
		}
	}
	return nil
}

func sendSMTP(ctx context.Context, settings ChannelSettings, message Message) error {
	address := net.JoinHostPort(settings.SMTPHost, strconv.Itoa(settings.SMTPPort))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var client *smtp.Client
	if settings.SMTPEncryption == "tls" {
		connection, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: settings.SMTPHost, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("SMTP TLS connection failed: %w", err)
		}
		client, err = smtp.NewClient(connection, settings.SMTPHost)
		if err != nil {
			_ = connection.Close()
			return fmt.Errorf("SMTP client initialization failed: %w", err)
		}
	} else {
		connection, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return fmt.Errorf("SMTP connection failed: %w", err)
		}
		client, err = smtp.NewClient(connection, settings.SMTPHost)
		if err != nil {
			_ = connection.Close()
			return fmt.Errorf("SMTP client initialization failed: %w", err)
		}
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return errors.New("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: settings.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
			_ = client.Close()
			return fmt.Errorf("SMTP STARTTLS failed: %w", err)
		}
	}
	defer func() { _ = client.Close() }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if settings.SMTPUsername != "" {
		if err := client.Auth(smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	if err := client.Mail(settings.SMTPFrom); err != nil {
		return fmt.Errorf("SMTP sender rejected: %w", err)
	}
	for _, recipient := range settings.SMTPRecipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("SMTP recipient rejected: %w", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP message rejected: %w", err)
	}
	subject := mimeHeader(message.Title)
	body := "From: " + settings.SMTPFrom + "\r\n" +
		"To: " + strings.Join(settings.SMTPRecipients, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + message.Content
	if _, err := io.WriteString(writer, body); err != nil {
		_ = writer.Close()
		return fmt.Errorf("SMTP message write failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP message completion failed: %w", err)
	}
	return client.Quit()
}

func mimeHeader(value string) string { return mimeQEncoding(value) }

func mimeQEncoding(value string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}
