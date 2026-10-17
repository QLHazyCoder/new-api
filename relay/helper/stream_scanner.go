package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10  // 64KB (64*1024)
	DefaultMaxScannerBufferSize = 128 << 20 // 64MB (64*1024*1024) default SSE buffer size
	DefaultPingInterval         = 10 * time.Second
	// streamWriteTimeout bounds a single blocked write to a slow client so the
	// unconditional wg.Wait() in cleanup can always finish. Without it, a slow
	// but connected client (full TCP buffer, no server WriteTimeout) could hang
	// the handler forever.
	streamWriteTimeout = 30 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

// NewStreamScanner shares relay scanner configuration. Callers buffering bounded
// task state may additionally cap a line without increasing the configured limit.
func NewStreamScanner(reader io.Reader, maxBytes ...int) *bufio.Scanner {
	limit := getScannerBufferSize()
	if len(maxBytes) > 0 && maxBytes[0] > 0 {
		limit = min(limit, maxBytes[0])
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, min(InitialScannerBufferSize, limit)), limit)
	return scanner
}

func copyCodexSSEHeaders(c *gin.Context, resp *http.Response) {
	if c == nil || c.Writer == nil || resp == nil {
		return
	}
	// codex
	for _, name := range []string{"X-Reasoning-Included", "X-Codex-Turn-State"} {
		values := resp.Header.Values(name)
		if !service.ShouldCopyUpstreamHeader(c, name, values) {
			continue
		}
		for _, value := range values {
			if value != "" {
				c.Writer.Header().Add(name, value)
			}
		}
	}
}

// ExtendWriteDeadline pushes the connection write deadline forward before each
// stream write. Best-effort: writers that don't support deadlines (e.g.
// httptest recorders) are silently ignored.
func ExtendWriteDeadline(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(streamWriteTimeout))
}

// Saturate only at the largest representable timer duration, not an operator-configured cap.
func clientGoneDrainDuration(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		return 0
	}
	const maxDuration = time.Duration(1<<63 - 1)
	const maxDurationSeconds = int64(maxDuration / time.Second)
	if int64(timeoutSeconds) > maxDurationSeconds {
		return maxDuration
	}
	return time.Duration(timeoutSeconds) * time.Second
}

func setDrainResultForScannerEnd(status *relaycommon.StreamStatus) {
	switch status.TerminalReason() {
	case relaycommon.StreamEndReasonScannerErr:
		status.SetDrainResult(relaycommon.StreamDrainResultScannerError)
	case relaycommon.StreamEndReasonHandlerStop, relaycommon.StreamEndReasonPanic, relaycommon.StreamEndReasonPingFail:
		status.SetDrainResult(relaycommon.StreamDrainResultHandlerStop)
	default:
		status.SetDrainResult(relaycommon.StreamDrainResultEnded)
	}
}

func finishTimedOutDrain(status *relaycommon.StreamStatus, scannerEndMu *sync.Mutex, scanDone <-chan struct{}, stopChan <-chan bool, requestErr error, result relaycommon.StreamDrainResult) {
	// Scanner completion and timeout classification share a lock so a terminal
	// event/read result wins when both become ready at the same time.
	scannerEndMu.Lock()
	defer scannerEndMu.Unlock()
	select {
	case <-scanDone:
		setDrainResultForScannerEnd(status)
	case <-stopChan:
		setDrainResultForScannerEnd(status)
	default:
		status.SetEndReason(relaycommon.StreamEndReasonClientGone, requestErr)
		if status.TerminalReason() != relaycommon.StreamEndReasonNone {
			setDrainResultForScannerEnd(status)
			return
		}
		status.SetDrainResult(result)
	}
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string, sr *StreamResult)) {
	if resp == nil || dataHandler == nil {
		return
	}

	// 无条件新建 StreamStatus
	info.StreamStatus = relaycommon.NewStreamStatus()

	ctx, cancel := context.WithCancel(context.Background())

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second

	var (
		stopChan     = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanDone     = make(chan struct{})
		scanner      = NewStreamScanner(resp.Body)
		ticker       = time.NewTicker(streamingTimeout)
		pingTicker   *time.Ticker
		writeMutex   sync.Mutex     // Mutex to protect concurrent writes
		wg           sync.WaitGroup // 用于等待所有 goroutine 退出
		cleanupOnce  sync.Once
		stopOnce     sync.Once
		scanDoneOnce sync.Once
		scannerEndMu sync.Mutex
	)
	signalScannerDone := func() {
		scanDoneOnce.Do(func() { close(scanDone) })
	}
	setScannerEndReason := func(reason relaycommon.StreamEndReason, err error) {
		scannerEndMu.Lock()
		if requestContextDone(c) {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
		}
		info.StreamStatus.SetEndReason(reason, err)
		signalScannerDone()
		scannerEndMu.Unlock()
	}
	setClientGone := func() {
		scannerEndMu.Lock()
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
		scannerEndMu.Unlock()
	}

	stop := func() {
		stopOnce.Do(func() {
			close(stopChan)
		})
	}

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	logger.LogDebug(c, "relay timeout seconds: %d", common.RelayTimeout)
	logger.LogDebug(c, "relay max idle conns: %d", common.RelayMaxIdleConns)
	logger.LogDebug(c, "relay max idle conns per host: %d", common.RelayMaxIdleConnsPerHost)
	logger.LogDebug(c, "streaming timeout seconds: %d", int64(streamingTimeout.Seconds()))
	logger.LogDebug(c, "ping interval seconds: %d", int64(pingInterval.Seconds()))

	cleanup := func() {
		cleanupOnce.Do(func() {
			cancel()
			stop()
			if resp.Body != nil {
				_ = resp.Body.Close()
			}

			ticker.Stop()
			if pingTicker != nil {
				pingTicker.Stop()
			}

			wg.Wait()
		})
	}
	// Ensure gin.Context is not returned to Gin's pool while any stream goroutine can still use it.
	defer cleanup()

	scanner.Split(bufio.ScanLines)
	copyCodexSSEHeaders(c, resp)
	SetEventStreamHeaders(c)

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(c, common.LogText("ping goroutine panic: %v", r))
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("ping panic: %v", r))
					stop()
				}
				logger.LogDebug(c, "ping goroutine exited")
				wg.Done()
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					if requestContextDone(c) {
						return
					}
					var err error
					func() {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						if !requestContextDone(c) {
							ExtendWriteDeadline(c)
						}
						err = PingData(c)
					}()
					if err != nil {
						if requestContextDone(c) {
							return
						}
						logger.LogError(c, common.LogText("ping data error: %s", err.Error()))
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
						return
					}
					logger.LogDebug(c, "ping data sent")
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					logger.LogError(c, common.LogText("ping goroutine max duration reached"))
					return
				}
			}
		})
	}

	dataChan := make(chan string, 10)

	wg.Add(1)
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogError(c, common.LogText("data handler goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", r))
			}
			stop()
			wg.Done()
		}()
		sr := newStreamResult(info.StreamStatus)
		for data := range dataChan {
			sr.reset()
			func() {
				writeMutex.Lock()
				defer writeMutex.Unlock()
				if requestContextDone(c) {
					// A queued frame may be processed after the client leaves;
					// preserve that transport fact before protocol callbacks
					// record a terminal response or stop the stream.
					setClientGone()
				}
				if !requestContextDone(c) {
					ExtendWriteDeadline(c)
				}
				dataHandler(data, sr)
			}()
			if sr.IsStopped() {
				return
			}
		}
	})

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogError(c, common.LogText("scanner goroutine panic: %v", r))
				setScannerEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("scanner panic: %v", r))
			}
			signalScannerDone()
			close(dataChan)
			stop()
			logger.LogDebug(c, "scanner goroutine exited")
			wg.Done()
		}()

		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			default:
			}

			ticker.Reset(streamingTimeout)
			data := scanner.Text()
			logger.LogDebug(c, "stream scanner data: %s", data)

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}
			if !strings.HasPrefix(data, "[DONE]") {
				info.SetFirstResponseTime()
				info.ReceivedResponseCount++

				select {
				case dataChan <- data:
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
			} else {
				setScannerEndReason(relaycommon.StreamEndReasonDone, nil)
				logger.LogDebug(c, "received [DONE], stopping scanner")
				return
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, common.LogText("scanner error: %s", err.Error()))
				setScannerEndReason(relaycommon.StreamEndReasonScannerErr, err)
				return
			}
		}
		setScannerEndReason(relaycommon.StreamEndReasonEOF, nil)
	})

	// Client cancellation is recorded independently from the first scanner end
	// reason. This preserves client_gone while drain_result describes what the
	// scanner observed before cleanup.
	clientGoneDrainTimeout := clientGoneDrainDuration(constant.ClientGoneDrainTimeout)

	// Main loop waits for normal stream completion, idle timeout, or client
	// cancellation. After cancellation the scanner and data handler remain in
	// this request's lifetime until the upstream ends or the absolute drain
	// timeout closes the body in cleanup.
mainLoop:
	select {
	case <-ticker.C:
		select {
		case <-scanDone:
			<-stopChan
		default:
			if requestContextDone(c) {
				setClientGone()
				if info.StreamStatus.TerminalReason() == relaycommon.StreamEndReasonNone {
					info.StreamStatus.SetDrainResult(relaycommon.StreamDrainResultIdleTimeout)
				} else {
					setDrainResultForScannerEnd(info.StreamStatus)
				}
			} else {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
			}
		}
	case <-stopChan:
		// EndReason already set by the goroutine that triggered stopChan
		if requestContextDone(c) {
			setClientGone()
			setDrainResultForScannerEnd(info.StreamStatus)
		}
	case <-c.Request.Context().Done():
		select {
		case <-scanDone:
			<-stopChan
			setClientGone()
			setDrainResultForScannerEnd(info.StreamStatus)
			break mainLoop
		default:
		}
		select {
		case <-stopChan:
			setClientGone()
			setDrainResultForScannerEnd(info.StreamStatus)
			break mainLoop
		default:
		}
		setClientGone()
		if clientGoneDrainTimeout <= 0 {
			if info.StreamStatus.TerminalReason() == relaycommon.StreamEndReasonNone {
				info.StreamStatus.SetDrainResult(relaycommon.StreamDrainResultDisabled)
			} else {
				setDrainResultForScannerEnd(info.StreamStatus)
			}
			break
		}
		drainTimer := time.NewTimer(clientGoneDrainTimeout)
		select {
		case <-stopChan:
			setDrainResultForScannerEnd(info.StreamStatus)
		case <-scanDone:
			<-stopChan
			setDrainResultForScannerEnd(info.StreamStatus)
		case <-ticker.C:
			finishTimedOutDrain(info.StreamStatus, &scannerEndMu, scanDone, stopChan, c.Request.Context().Err(), relaycommon.StreamDrainResultIdleTimeout)
		case <-drainTimer.C:
			finishTimedOutDrain(info.StreamStatus, &scannerEndMu, scanDone, stopChan, c.Request.Context().Err(), relaycommon.StreamDrainResultTimeout)
		}
		drainTimer.Stop()
	}

	cleanup()
	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, common.LogText("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, common.LogText("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
}
