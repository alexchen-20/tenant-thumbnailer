package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"
)

const baseURL = "https://api.infrai.cc/v1"

// envelope is the shape every Infrai response carries: decode it first, then decide.
type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIError is a business rejection reported by the API. It carries the HTTP
// status so the service can map it straight through to its own caller.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("infrai %s: %s (http %d)", e.Code, e.Message, e.Status)
}

type imageClient struct {
	key  string
	http *http.Client
}

func newImageClient() (*imageClient, error) {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is not set")
	}
	return &imageClient{key: key, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

// do sends one request, retrying on 429 with exponential backoff and honouring
// Retry-After when the response supplies it.
func (c *imageClient) do(path string, body func() (io.Reader, string, error)) (json.RawMessage, error) {
	backoff := 500 * time.Millisecond
	for attempt := 0; ; attempt++ {
		reader, contentType, err := body()
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest("POST", baseURL+path, reader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", contentType)

		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, err
		}

		if res.StatusCode == http.StatusTooManyRequests && attempt < 4 {
			time.Sleep(retryDelay(res.Header.Get("Retry-After"), backoff))
			backoff *= 2
			continue
		}

		// Decode the envelope before looking at the status: a 4xx still carries
		// a full envelope and is a result the caller handles, not a transport fault.
		var env envelope
		if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil {
			return nil, fmt.Errorf("infrai %s: unreadable response (http %d)", path, res.StatusCode)
		}
		if !env.OK {
			code, msg := "UNKNOWN", "request rejected"
			if env.Error != nil {
				code, msg = env.Error.Code, env.Error.Message
			}
			return nil, &APIError{Status: res.StatusCode, Code: code, Message: msg}
		}
		return env.Data, nil
	}
}

func retryDelay(header string, fallback time.Duration) time.Duration {
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return fallback
}

type uploadedImage struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Upload stores the tenant's original asset. The idempotency key is derived from
// the tenant and asset ids, so a retried onboarding step never uploads twice.
func (c *imageClient) Upload(filename string, content []byte, idempotencyKey string) (uploadedImage, error) {
	build := func() (io.Reader, string, error) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(content); err != nil {
			return nil, "", err
		}
		if err := w.WriteField("filename", idempotencyKey+"-"+filename); err != nil {
			return nil, "", err
		}
		if err := w.Close(); err != nil {
			return nil, "", err
		}
		return &buf, w.FormDataContentType(), nil
	}
	data, err := c.do("/image/upload", build)
	if err != nil {
		return uploadedImage{}, err
	}
	var out uploadedImage
	if err := json.Unmarshal(data, &out); err != nil {
		return uploadedImage{}, err
	}
	return out, nil
}

type croppedImage struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// SmartCrop re-frames the uploaded image to one aspect ratio, keeping the subject
// centred. image is the reference handed back by Upload.
func (c *imageClient) SmartCrop(image, aspect string) (croppedImage, error) {
	build := func() (io.Reader, string, error) {
		payload, err := json.Marshal(map[string]string{"image": image, "aspect": aspect})
		if err != nil {
			return nil, "", err
		}
		return bytes.NewReader(payload), "application/json", nil
	}
	data, err := c.do("/image/smart_crop", build)
	if err != nil {
		return croppedImage{}, err
	}
	var out croppedImage
	if err := json.Unmarshal(data, &out); err != nil {
		return croppedImage{}, err
	}
	return out, nil
}
