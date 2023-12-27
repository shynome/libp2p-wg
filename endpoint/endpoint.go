package endpoint

import (
	"net/netip"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"golang.zx2c4.com/wireguard/conn"
)

var ProtocolID = protocol.ID("/wireguard/tunnel")

type Sender interface {
	Send(buf []byte) error
}

type Endpoint struct {
	network.Stream
}

var _ conn.Endpoint = (*Endpoint)(nil)

func New(stream network.Stream) *Endpoint {
	return &Endpoint{
		Stream: stream,
	}
}

// used for mac2 cookie calculations
func (ep *Endpoint) DstToBytes() []byte {
	if ep.Stream == nil {
		return nil
	}
	return []byte(ep.Stream.ID())
}

func (ep *Endpoint) DstToString() string {
	if ep.Stream == nil {
		return "[fdd9:f800::]:80"
	}
	conn := ep.Stream.Conn()
	return conn.RemoteMultiaddr().String()
}

func (ep *Endpoint) SrcToString() string { return "" }
func (ep *Endpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (ep *Endpoint) SrcIP() netip.Addr   { return netip.Addr{} }
func (ep *Endpoint) ClearSrc()           {}
