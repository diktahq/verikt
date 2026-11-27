package engineclient

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadMessage_RejectsOversizedPayload(t *testing.T) {
	// Send a length prefix of 128 MiB — exceeds maxMsgSize (64 MiB).
	var buf bytes.Buffer
	oversizedLen := uint32(128 * 1024 * 1024)
	_ = binary.Write(&buf, binary.LittleEndian, oversizedLen)

	_, err := readMessage(&buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestReadMessage_AcceptsReasonablePayload(t *testing.T) {
	// A valid-sized length prefix shouldn't error on size check.
	// It will error on protobuf decode (garbage data), but not on size.
	var buf bytes.Buffer
	reasonableLen := uint32(1024)
	_ = binary.Write(&buf, binary.LittleEndian, reasonableLen)
	// Write enough garbage bytes to satisfy the read.
	buf.Write(make([]byte, 1024))

	_, err := readMessage(&buf)
	// Should fail on protobuf decode, NOT on size check.
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "too large")
}

func TestFrameLength_RejectsPayloadAboveMaxMsgSize(t *testing.T) {
	_, err := frameLength(maxMsgSize + 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestFrameLength_EncodesPayloadAtMaxMsgSize(t *testing.T) {
	n, err := frameLength(maxMsgSize)
	assert.NoError(t, err)
	assert.Equal(t, uint32(maxMsgSize), n)
}
