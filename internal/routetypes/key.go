package routetypes

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

type Key struct {

	// first member must be a prefix u32 wide
	// rest can are arbitrary
	Prefixlen uint32
	IP        net.IP
}

func (l Key) Bytes() []byte {
	output := make([]byte, 8)
	binary.LittleEndian.PutUint32(output[0:4], l.Prefixlen)
	copy(output[4:], l.IP.To4())

	return output
}

func (l *Key) Unpack(b []byte) error {
	if len(b) != 8 {
		return errors.New("too short")
	}

	l.Prefixlen = binary.LittleEndian.Uint32(b[:4])
	l.IP = b[4:]

	return nil
}

func (l Key) String() string {
	return fmt.Sprintf("%s/%d", l.IP.String(), l.Prefixlen)
}

func lookupRuleType(t uint16) string {
	switch t {
	case RANGE:
		return "range"
	case SINGLE:
		return "single"
	case STOP:
		return "stop"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func lookupProtocol(t uint16) string {
	switch t {
	case TCP:
		return "tcp"
	case UDP:
		return "udp"
	case ICMP:
		return "icmp"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}
