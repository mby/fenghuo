package agent

import (
	"bytes"
	"encoding/binary"
)

func echoRequest(payload []byte) []byte {
	msg := make([]byte, 8+len(payload))
	msg[0] = 8
	binary.BigEndian.PutUint16(msg[6:8], 1)
	copy(msg[8:], payload)
	var sum uint32
	for i := 0; i+1 < len(msg); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(msg[i : i+2]))
	}
	if len(msg)%2 != 0 {
		sum += uint32(msg[len(msg)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	binary.BigEndian.PutUint16(msg[2:4], ^uint16(sum))
	return msg
}

func validEcho(reply, request []byte) bool {
	// Linux assigns the identifier for datagram ping sockets. Match our random
	// payload and sequence instead, so delayed or unrelated replies cannot win.
	return len(reply) == len(request) && len(reply) >= 8 && reply[0] == 0 && reply[1] == 0 && bytes.Equal(reply[6:], request[6:])
}
