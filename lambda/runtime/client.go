package runtime

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

type Client struct {
	BaseURL    *url.URL
	UserAgent  string
	httpClient *http.Client
}

const RuntimeInitErrorPath = "/2018-06-01/runtime/init/error"
const NextEventPath = "/2018-06-01/runtime/invocation/next"
const ResponsePath = "/2018-06-01/runtime/invocation/%s/response"
const ResponseErrorPath = "/2018-06-01/runtime/invocation/%s/error"

const UserAgent = "OverlazyRuntimeGoClient"

func New(client *http.Client) *Client {
	return &Client{BaseURL: &url.URL{Host: os.Getenv("AWS_LAMBDA_RUNTIME_API"), Scheme: "http"}, UserAgent: UserAgent, httpClient: client}
}

func (c *Client) GetInvocation() (string, int64, error) {
	rel := &url.URL{Path: NextEventPath}
	u := c.BaseURL.ResolveReference(rel)
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.do(req)
	if err != nil {
		return "", 0, err
	}

	n, err := strconv.ParseInt(resp.Header.Get("Lambda-Runtime-Deadline-Ms"), 10, 64)
	if err != nil {
		return "", 0, err
	}

	return resp.Header.Get("Lambda-Runtime-Aws-Request-Id"), n, err
}

func (c *Client) PostResponse(requestId string, body string) error {
	rel := &url.URL{Path: fmt.Sprintf(ResponsePath, requestId)}
	u := c.BaseURL.ResolveReference(rel)

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer([]byte(body)))
	req.Header.Set("Content-Type", "text/plain")
	_, err = c.do(req)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) PostError(requestId string, body string) error {
	rel := &url.URL{Path: fmt.Sprintf(ResponseErrorPath, requestId)}
	u := c.BaseURL.ResolveReference(rel)

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer([]byte(body)))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Lambda-Runtime-Function-Error-Type", "Unhandled")

	_, err = c.do(req)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) PostInitializationError(body string) error {
	rel := &url.URL{Path: RuntimeInitErrorPath}
	u := c.BaseURL.ResolveReference(rel)

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer([]byte(body)))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Lambda-Runtime-Function-Error-Type", "Unhandled")

	_, err = c.do(req)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return resp, err
}
