// package main
package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const MAX_TOPIC_LENGTH = 265
const PROTOCOLL_ID = 0x6201

// encodePacket encodes a topic name and binary payload into a binary packet.
func EncodePacket(topic string, payload []byte) ([]byte, error) {
	var buf bytes.Buffer

	// Write protocol ID as a uint16
	if err := binary.Write(&buf, binary.BigEndian, uint16(PROTOCOLL_ID)); err != nil {
		return nil, err
	}

	// Write the length of the topic as a uint16
	if err := binary.Write(&buf, binary.BigEndian, uint16(len(topic))); err != nil {
		return nil, err
	}

	// Write the topic as a string
	if _, err := buf.WriteString(topic); err != nil {
		return nil, err
	}

	// Write the payload length as a uint32
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, err
	}

	// Write the payload
	if _, err := buf.Write(payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decodePacket decodes a binary packet into a topic name and binary payload.
func DecodePacket(packet []byte) (string, []byte, error) {
	buf := bytes.NewReader(packet)

	var protocolId uint16
	if err := binary.Read(buf, binary.BigEndian, &protocolId); err != nil {
		return "", nil, err
	}

	if protocolId != PROTOCOLL_ID {
		return "", nil, fmt.Errorf("unknown protocoll")
	}

	// Read the length of the topic
	var topicLen uint16
	if err := binary.Read(buf, binary.BigEndian, &topicLen); err != nil {
		return "", nil, err
	}

	if topicLen <= 0 || topicLen > MAX_TOPIC_LENGTH {
		return "", nil, fmt.Errorf("invalid toplc length")
	}

	// Read the topic
	topic := make([]byte, topicLen)
	if _, err := buf.Read(topic); err != nil {
		return "", nil, err
	}

	// Read the payload length
	var payloadLen uint32
	if err := binary.Read(buf, binary.BigEndian, &payloadLen); err != nil {
		return "", nil, err
	}

	// Read the payload
	payload := make([]byte, payloadLen)
	if _, err := buf.Read(payload); err != nil {
		return "", nil, err
	}

	return string(topic), payload, nil
}
