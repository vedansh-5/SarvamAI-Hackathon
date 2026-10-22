package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	Region       = "ap-south-1"
	SettingsPath = "/vans/server/"
	EnvMailFrom  = "VANS_FEEDBACK_FROM"
	EnvMailTo    = "VANS_FEEDBACK_TO"
)

func OnAWS(ctx context.Context, table string) (*Dynamo, Mailer, Sender, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(Region))
	if err != nil {
		return nil, nil, nil, err
	}
	ledger := &Dynamo{Client: dynamodb.NewFromConfig(cfg), Table: table}
	client := sesv2.NewFromConfig(cfg)
	var mail Mailer
	var send Sender
	if from, to := strings.TrimSpace(os.Getenv(EnvMailFrom)), strings.TrimSpace(os.Getenv(EnvMailTo)); from != "" && to != "" {
		mail = &SES{Client: client, From: from, To: to}
	}
	if from := strings.TrimSpace(os.Getenv(EnvJoinFrom)); from != "" {
		send = &SES{Client: client, From: from}
	}
	return ledger, mail, send, nil
}

type SESAPI interface {
	SendEmail(ctx context.Context, in *sesv2.SendEmailInput, opts ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

type SES struct {
	Client SESAPI
	From   string
	To     string
}

func (m *SES) Mail(ctx context.Context, subject, body string) error {
	return m.Send(ctx, m.To, subject, body)
}

func (m *SES) Send(ctx context.Context, to, subject, body string) error {
	_, err := m.Client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.From),
		Destination:      &sestypes.Destination{ToAddresses: []string{to}},
		Content: &sestypes.EmailContent{Simple: &sestypes.Message{
			Subject: &sestypes.Content{Data: aws.String(subject), Charset: aws.String("UTF-8")},
			Body:    &sestypes.Body{Text: &sestypes.Content{Data: aws.String(body), Charset: aws.String("UTF-8")}},
		}},
	})
	return err
}

type SSMAPI interface {
	GetParametersByPath(ctx context.Context, in *ssm.GetParametersByPathInput, opts ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
}

func LoadSettings(ctx context.Context, client SSMAPI) (int, error) {
	loaded := 0
	var next *string
	for {
		out, err := client.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
			Path: aws.String(SettingsPath), WithDecryption: aws.Bool(true), NextToken: next,
		})
		if err != nil {
			return loaded, fmt.Errorf("server: read settings under %s: %w", SettingsPath, err)
		}
		for _, p := range out.Parameters {
			if p.Name == nil || p.Value == nil {
				continue
			}
			if err := os.Setenv(path.Base(*p.Name), *p.Value); err != nil {
				return loaded, err
			}
			loaded++
		}
		if out.NextToken == nil {
			return loaded, nil
		}
		next = out.NextToken
	}
}

func FunctionURL(h http.Handler) func(ctx context.Context, in events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	return func(ctx context.Context, in events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
		body := []byte(in.Body)
		if in.IsBase64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(in.Body)
			if err != nil {
				return events.LambdaFunctionURLResponse{StatusCode: http.StatusBadRequest}, nil
			}
			body = decoded
		}
		target := in.RawPath
		if in.RawQueryString != "" {
			target += "?" + in.RawQueryString
		}
		req, err := http.NewRequestWithContext(ctx, in.RequestContext.HTTP.Method, target, bytes.NewReader(body))
		if err != nil {
			return events.LambdaFunctionURLResponse{StatusCode: http.StatusBadRequest}, nil
		}
		for k, v := range in.Headers {
			req.Header.Set(k, v)
		}
		req.RemoteAddr = in.RequestContext.HTTP.SourceIP
		req.Host = in.RequestContext.DomainName

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		headers := map[string]string{}
		for k, v := range rec.Header() {
			headers[k] = strings.Join(v, ", ")
		}
		return events.LambdaFunctionURLResponse{StatusCode: rec.Code, Headers: headers, Body: rec.Body.String()}, nil
	}
}
