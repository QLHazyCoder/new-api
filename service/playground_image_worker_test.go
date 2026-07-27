package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type playgroundImageRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip playgroundImageRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func tinyPlaygroundPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	value := image.NewRGBA(image.Rect(0, 0, 2, 2))
	value.Set(0, 0, color.RGBA{G: 255, A: 255})
	require.NoError(t, png.Encode(&buffer, value))
	return buffer.Bytes()
}

func TestSavePlaygroundImageExecutionResultStoresOnlyLocalFile(t *testing.T) {
	t.Setenv(playgroundImageStorageEnv, t.TempDir())
	pngBytes := tinyPlaygroundPNG(t)
	responseBody, err := common.Marshal(map[string]any{
		"data": []map[string]any{{
			"b64_json":  base64.StdEncoding.EncodeToString(pngBytes),
			"mime_type": "image/png",
		}},
	})
	require.NoError(t, err)
	task := &model.PlaygroundImageTask{
		TaskID:    "imgtask_test",
		UserID:    7,
		CreatedAt: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC).Unix(),
	}

	relativePath, mimeType, size, err := savePlaygroundImageExecutionResult(
		context.Background(),
		task,
		&PlaygroundImageExecutionResult{
			StatusCode: http.StatusOK,
			Body:       responseBody,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "image/png", mimeType)
	assert.EqualValues(t, len(pngBytes), size)
	assert.Contains(t, relativePath, task.TaskID)
	assert.False(t, strings.Contains(relativePath, "http"))

	file, err := OpenPlaygroundImageResult(relativePath)
	require.NoError(t, err)
	stored, err := io.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	assert.Equal(t, pngBytes, stored)
}

func TestSavePlaygroundImageExecutionResultAcceptsDataURLInURLField(t *testing.T) {
	t.Setenv(playgroundImageStorageEnv, t.TempDir())
	pngBytes := tinyPlaygroundPNG(t)
	responseBody, err := common.Marshal(map[string]any{
		"data": []map[string]any{{
			"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes),
		}},
	})
	require.NoError(t, err)
	task := &model.PlaygroundImageTask{
		TaskID:    "imgtask_data_url",
		UserID:    1,
		CreatedAt: time.Now().Unix(),
	}

	path, mimeType, _, err := savePlaygroundImageExecutionResult(
		context.Background(),
		task,
		&PlaygroundImageExecutionResult{StatusCode: http.StatusOK, Body: responseBody},
	)
	require.NoError(t, err)
	assert.Equal(t, "image/png", mimeType)
	require.NoError(t, RemovePlaygroundImageResult(path))
}

func TestPlaygroundImageStorageRejectsSVGAndPathTraversal(t *testing.T) {
	t.Setenv(playgroundImageStorageEnv, t.TempDir())
	_, err := validatePlaygroundImageBytes(
		[]byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"),
		"image/svg+xml",
	)
	require.Error(t, err)

	_, err = resolvePlaygroundImagePath("../outside.png")
	require.Error(t, err)
}

func TestPlaygroundImageDownloadErrorDoesNotExposeUpstreamHost(t *testing.T) {
	originalClient := ssrfProtectedHTTPClient
	ssrfProtectedHTTPClient = &http.Client{
		Transport: playgroundImageRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial tcp 127.0.0.1: connection refused")
		}),
	}
	t.Cleanup(func() {
		ssrfProtectedHTTPClient = originalClient
	})

	_, _, err := downloadPlaygroundImage(context.Background(), "http://127.0.0.1/private-image.png")
	require.EqualError(t, err, "failed to download generated image")
	assert.NotContains(t, err.Error(), "127.0.0.1")
}

func TestPlaygroundImageRelayErrorRedactsUpstreamURL(t *testing.T) {
	body := []byte(`{"error":{"message":"fetch https://pool.example/images/private.png failed","code":"https://pool.example/code"}}`)
	message, code := parsePlaygroundImageRelayError(
		&PlaygroundImageExecutionResult{StatusCode: http.StatusBadGateway, Body: body},
		nil,
	)
	assert.Equal(t, "fetch [upstream URL redacted] failed", message)
	assert.Equal(t, "[upstream URL redacted]", code)
}
