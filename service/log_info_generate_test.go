package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendStreamStatusKeepsTransportAndProtocolFieldsIndependent(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonClientGone, assert.AnError)
	status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	status.MarkCompleted()
	status.SetDrainResult(relaycommon.StreamDrainResultEnded)

	other := model.NewLogOther()
	appendStreamStatus(&relaycommon.RelayInfo{IsStream: true, StreamStatus: status}, other)

	streamStatus, ok := other.Snapshot()["stream_status"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "client_gone", streamStatus["end_reason"])
	assert.Equal(t, "completed", streamStatus["response_status"])
	assert.Equal(t, "ended", streamStatus["drain_result"])
}

func TestAppendStreamStatusOmitsDrainResultForNormalStream(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonDone, nil)

	other := model.NewLogOther()
	appendStreamStatus(&relaycommon.RelayInfo{IsStream: true, StreamStatus: status}, other)

	streamStatus, ok := other.Snapshot()["stream_status"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "done", streamStatus["end_reason"])
	assert.NotContains(t, streamStatus, "drain_result")
}
