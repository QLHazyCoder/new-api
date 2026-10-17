package helper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type readStartBody struct {
	io.ReadCloser
	started chan struct{}
	once    sync.Once
}

func TestClientGoneDrainDuration(t *testing.T) {
	assert.Zero(t, clientGoneDrainDuration(0))
	assert.Equal(t, 60*time.Second, clientGoneDrainDuration(60))

	maxDuration := time.Duration(1<<63 - 1)
	maxSeconds := int64(maxDuration / time.Second)
	maxInt := uint64(^uint(0) >> 1)
	if uint64(maxSeconds+1) <= maxInt {
		assert.Equal(t, maxDuration, clientGoneDrainDuration(int(maxSeconds+1)))
	}
}

func (b *readStartBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	return b.ReadCloser.Read(p)
}

func init() {
	gin.SetMode(gin.TestMode)
	if constant.StreamingTimeout == 0 {
		constant.StreamingTimeout = 30
	}
}

func setupStreamTest(t *testing.T, body io.Reader) (*gin.Context, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{
		Body: io.NopCloser(body),
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	return c, resp, info
}

func buildSSEBody(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "data: {\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}\n", i, i)
	}
	b.WriteString("data: [DONE]\n")
	return b.String()
}

// ---------- Basic correctness ----------

func TestStreamScannerHandler_NilInputs(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	StreamScannerHandler(c, nil, info, func(data string, sr *StreamResult) {})
	StreamScannerHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(""))}, info, nil)
}

func TestNewStreamScanner_AllowsLargeStreamLine(t *testing.T) {
	oldBufferMB := constant.StreamScannerMaxBufferMB
	constant.StreamScannerMaxBufferMB = 1
	t.Cleanup(func() {
		constant.StreamScannerMaxBufferMB = oldBufferMB
	})

	payload := strings.Repeat("x", 128<<10)
	scanner := NewStreamScanner(strings.NewReader("data: " + payload + "\n"))
	scanner.Split(bufio.ScanLines)

	require.True(t, scanner.Scan())
	assert.Equal(t, "data: "+payload, scanner.Text())
	require.NoError(t, scanner.Err())
}

func TestStreamScannerHandler_EmptyBody(t *testing.T) {
	t.Parallel()

	c, resp, info := setupStreamTest(t, strings.NewReader(""))

	var called atomic.Bool
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		called.Store(true)
	})

	assert.False(t, called.Load(), "handler should not be called for empty body")
}

func TestStreamScannerHandler_1000Chunks(t *testing.T) {
	t.Parallel()

	const numChunks = 1000
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(numChunks), count.Load())
	assert.Equal(t, numChunks, info.ReceivedResponseCount)
}

func TestStreamScannerHandler_OrderPreserved(t *testing.T) {
	t.Parallel()

	const numChunks = 500
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var mu sync.Mutex
	received := make([]string, 0, numChunks)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		mu.Lock()
		received = append(received, data)
		mu.Unlock()
	})

	require.Equal(t, numChunks, len(received))
	for i := range numChunks {
		expected := fmt.Sprintf("{\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}", i, i)
		assert.Equal(t, expected, received[i], "chunk %d out of order", i)
	}
}

func TestStreamScannerHandler_DoneStopsScanner(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(50) + "data: should_not_appear\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(50), count.Load(), "data after [DONE] must not be processed")
}

func TestStreamScannerHandler_StopStopsStream(t *testing.T) {
	t.Parallel()

	const numChunks = 200
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	const stopAt int64 = 50
	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= stopAt {
			sr.Stop(fmt.Errorf("fatal at %d", n))
		}
	})

	assert.Equal(t, stopAt, count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
}

func TestStreamScannerHandler_SkipsNonDataLines(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString(": comment line\n")
	b.WriteString("event: message\n")
	b.WriteString("id: 12345\n")
	b.WriteString("retry: 5000\n")
	for i := range 100 {
		fmt.Fprintf(&b, "data: payload_%d\n", i)
		b.WriteString(": interleaved comment\n")
	}
	b.WriteString("data: [DONE]\n")

	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(100), count.Load())
}

func TestStreamScannerHandler_DataWithExtraSpaces(t *testing.T) {
	t.Parallel()

	body := "data:   {\"trimmed\":true}  \ndata: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var got string
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		got = data
	})

	assert.Equal(t, "{\"trimmed\":true}", got)
}

// TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns covers the
// explicitly disabled drain mode. The default positive timeout is tested below.
func TestStreamScannerHandler_ClientCancelWithDrainDisabledReturns(t *testing.T) {
	oldDrainTimeout := constant.ClientGoneDrainTimeout
	constant.ClientGoneDrainTimeout = 0
	t.Cleanup(func() { constant.ClientGoneDrainTimeout = oldDrainTimeout })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	firstHandled := make(chan struct{})
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
			_ = StringData(c, data)
			if data == "first" {
				close(firstHandled)
			}
		})
		close(done)
	}()

	_, err := fmt.Fprint(pw, "data: first\n")
	require.NoError(t, err)

	select {
	case <-firstHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk")
	}

	cancel()

	// The handler must return without any further upstream input: cleanup
	// closes resp.Body, which unblocks the scanner goroutine.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect")
	}

	// Upstream read side must be closed so the provider stops generating
	// (and billing) for a request nobody is listening to.
	_, err = fmt.Fprint(pw, "data: second\n")
	require.ErrorIs(t, err, io.ErrClosedPipe, "upstream body should be closed after client disconnect")

	assert.Equal(t, int64(1), count.Load(), "no chunk after disconnect should be processed")
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, relaycommon.StreamDrainResultDisabled, info.StreamStatus.OutcomeSnapshot().DrainResult)

	body := recorder.Body.String()
	assert.Contains(t, body, "first")
	assert.NotContains(t, body, "second")
}

func TestStreamScannerHandler_DrainCompletesAfterClientCancel(t *testing.T) {
	oldDrainTimeout := constant.ClientGoneDrainTimeout
	constant.ClientGoneDrainTimeout = 1
	t.Cleanup(func() { constant.ClientGoneDrainTimeout = oldDrainTimeout })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}

	var received []string
	var receivedMu sync.Mutex
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: pr}, info, func(data string, sr *StreamResult) {
			receivedMu.Lock()
			received = append(received, data)
			receivedMu.Unlock()
			_ = StringData(c, data)
		})
		close(done)
	}()

	_, err := fmt.Fprintln(pw, "data: first")
	require.NoError(t, err)
	cancel()
	require.Eventually(t, func() bool {
		return info.StreamStatus.OutcomeSnapshot().EndReason == relaycommon.StreamEndReasonClientGone
	}, time.Second, time.Millisecond)
	_, err = fmt.Fprintln(pw, "data: terminal")
	require.NoError(t, err, "upstream body should remain readable during the bounded drain")
	_, err = fmt.Fprintln(pw, "data: [DONE]")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish after the upstream stream ended")
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()
	assert.Equal(t, []string{"first", "terminal"}, received)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.TerminalReason())
	assert.Equal(t, relaycommon.StreamDrainResultEnded, info.StreamStatus.OutcomeSnapshot().DrainResult)
	assert.NotContains(t, recorder.Body.String(), "terminal", "drained events must not be written after client disconnect")
}

func TestStreamDataWritersIgnoreClientGone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	defer cancel()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)

	assert.NoError(t, StringData(c, "after-disconnect"))
	assert.NoError(t, ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.completed"}, `{"type":"response.completed"}`))
	assert.NoError(t, ObjectData(c, map[string]string{"event": "after-disconnect"}))
	assert.NoError(t, ClaudeData(c, dto.ClaudeResponse{}))
	ClaudeChunkData(c, dto.ClaudeResponse{}, `{"type":"message_delta"}`)
	assert.Empty(t, recorder.Body.String(), "disconnected requests must not write downstream data")
}

func TestStreamScannerHandler_DrainStopsAtAbsoluteTimeout(t *testing.T) {
	oldDrainTimeout := constant.ClientGoneDrainTimeout
	constant.ClientGoneDrainTimeout = 1
	t.Cleanup(func() { constant.ClientGoneDrainTimeout = oldDrainTimeout })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	defer cancel()
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: pr}, info, func(data string, sr *StreamResult) {})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not clean up after drain was bounded")
	}
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, relaycommon.StreamDrainResultTimeout, info.StreamStatus.OutcomeSnapshot().DrainResult)
}

func TestStreamScannerHandler_DrainCompletesAtEOF(t *testing.T) {
	oldDrainTimeout := constant.ClientGoneDrainTimeout
	constant.ClientGoneDrainTimeout = 1
	t.Cleanup(func() { constant.ClientGoneDrainTimeout = oldDrainTimeout })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: pr}, info, func(data string, sr *StreamResult) {})
		close(done)
	}()

	cancel()
	_, err := fmt.Fprintln(pw, "data: terminal")
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish after EOF")
	}
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.TerminalReason())
	assert.Equal(t, relaycommon.StreamDrainResultEnded, info.StreamStatus.OutcomeSnapshot().DrainResult)
}

func TestStreamScannerHandler_DrainRecordsScannerError(t *testing.T) {
	oldDrainTimeout := constant.ClientGoneDrainTimeout
	constant.ClientGoneDrainTimeout = 1
	t.Cleanup(func() { constant.ClientGoneDrainTimeout = oldDrainTimeout })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	body := &readStartBody{ReadCloser: pr, started: make(chan struct{})}
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: body}, info, func(data string, sr *StreamResult) {})
		close(done)
	}()
	<-body.started
	cancel()
	require.Eventually(t, func() bool {
		return info.StreamStatus.OutcomeSnapshot().EndReason == relaycommon.StreamEndReasonClientGone
	}, time.Second, time.Millisecond)
	require.NoError(t, pw.CloseWithError(errors.New("upstream read failed")))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after upstream read error")
	}

	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, relaycommon.StreamEndReasonScannerErr, info.StreamStatus.TerminalReason())
	assert.Equal(t, relaycommon.StreamDrainResultScannerError, info.StreamStatus.OutcomeSnapshot().DrainResult)
}

func TestStreamScannerHandler_DrainRecordsHandlerStop(t *testing.T) {
	oldDrainTimeout := constant.ClientGoneDrainTimeout
	constant.ClientGoneDrainTimeout = 1
	t.Cleanup(func() { constant.ClientGoneDrainTimeout = oldDrainTimeout })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	callbackStarted := make(chan struct{})
	allowStop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: pr}, info, func(data string, sr *StreamResult) {
			close(callbackStarted)
			<-allowStop
			sr.Stop(errors.New("handler stopped"))
		})
		close(done)
	}()

	_, err := fmt.Fprintln(pw, "data: first")
	require.NoError(t, err)
	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("handler callback did not start")
	}
	cancel()
	require.Eventually(t, func() bool {
		return info.StreamStatus.OutcomeSnapshot().EndReason == relaycommon.StreamEndReasonClientGone
	}, time.Second, time.Millisecond)
	close(allowStop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish after handler stop")
	}
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.TerminalReason())
	assert.Equal(t, relaycommon.StreamDrainResultHandlerStop, info.StreamStatus.OutcomeSnapshot().DrainResult)
}

func TestFinishTimedOutDrainPrefersReadyScannerTerminal(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	scanDone := make(chan struct{})
	close(scanDone)
	stopChan := make(chan bool)
	close(stopChan)
	var scannerEndMu sync.Mutex

	finishTimedOutDrain(status, &scannerEndMu, scanDone, stopChan, errors.New("context canceled"), relaycommon.StreamDrainResultTimeout)

	assert.Equal(t, relaycommon.StreamEndReasonDone, status.OutcomeSnapshot().EndReason)
	assert.Equal(t, relaycommon.StreamDrainResultEnded, status.OutcomeSnapshot().DrainResult)
}

func TestStreamScannerHandler_DrainIdleTimeout(t *testing.T) {
	oldDrainTimeout := constant.ClientGoneDrainTimeout
	oldStreamingTimeout := constant.StreamingTimeout
	constant.ClientGoneDrainTimeout = 3
	constant.StreamingTimeout = 1
	t.Cleanup(func() {
		constant.ClientGoneDrainTimeout = oldDrainTimeout
		constant.StreamingTimeout = oldStreamingTimeout
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: pr}, info, func(data string, sr *StreamResult) {})
		close(done)
	}()
	_, err := fmt.Fprintln(pw, "data: first")
	require.NoError(t, err)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not finish after streaming idle timeout")
	}
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, relaycommon.StreamDrainResultIdleTimeout, info.StreamStatus.OutcomeSnapshot().DrainResult)
}

// ---------- Ping tests ----------

func TestStreamScannerHandler_PingSentDuringSlowUpstream(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := range 4 {
			fmt.Fprintf(pw, "data: chunk_%d\n", i)
			time.Sleep(400 * time.Millisecond)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream to finish")
	}

	assert.Equal(t, int64(4), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.GreaterOrEqual(t, pingCount, 1,
		"expected at least 1 ping during slow stream with 1s interval; got %d", pingCount)
}

func TestStreamScannerHandler_PingDisabledByRelayInfo(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(buildSSEBody(5)))}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	assert.Equal(t, int64(5), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.Equal(t, 0, pingCount, "pings should be disabled when DisablePing=true")
}

// ---------- StreamStatus integration ----------

func TestStreamScannerHandler_StreamStatus_DoneReason(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Nil(t, info.StreamStatus.EndError)
	assert.True(t, info.StreamStatus.IsNormalEnd())
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_EOFWithoutDone(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := range 5 {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_StreamStatus_HandlerStop(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(100)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 10 {
			sr.Stop(fmt.Errorf("stop at 10"))
		}
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_HandlerDone(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(20)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 5 {
			sr.Done()
		}
	})

	assert.Equal(t, int64(5), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_Timeout(t *testing.T) {
	// Not parallel: modifies global constant.StreamingTimeout
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	pr, pw := io.Pipe()
	go func() {
		fmt.Fprint(pw, "data: {\"id\":1}\n")
		time.Sleep(2 * time.Second)
		pw.Close()
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream timeout")
	}

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_StreamStatus_SoftErrors(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("soft error for chunk"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_MultipleErrorsPerChunk(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("error A"))
		sr.Error(fmt.Errorf("error B"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_ErrorThenStop(t *testing.T) {
	t.Parallel()

	// Use a large body without [DONE] to avoid race between scanner's [DONE]
	// and handler's Stop on the sync.Once EndReason.
	var b strings.Builder
	for i := range 100 {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
		sr.Error(fmt.Errorf("soft error"))
		sr.Stop(fmt.Errorf("fatal"))
	})

	assert.Equal(t, int64(1), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.Equal(t, 2, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_InitializedIfNil(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(1)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	assert.Nil(t, info.StreamStatus)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.NotNil(t, info.StreamStatus)
}

func TestStreamScannerHandler_StreamStatus_ReplacesPreInitialized(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RecordError("pre-existing error")

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 0, info.StreamStatus.TotalErrorCount())
}

func TestNewStreamScannerCallerLimit(t *testing.T) {
	// The smaller buffer must actually constrain a line; a preallocated 64 KiB
	// buffer would otherwise bypass this caller's 1 KiB limit in bufio.Scanner.
	scanner := NewStreamScanner(strings.NewReader(strings.Repeat("x", 2048)+"\n"), 1024)
	assert.False(t, scanner.Scan())
	require.Error(t, scanner.Err())
	scanner = NewStreamScanner(strings.NewReader("data: ok\n"), 1024)
	require.True(t, scanner.Scan())
	assert.Equal(t, "data: ok", scanner.Text())
	require.NoError(t, scanner.Err())
}
