package endpoint

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"golang.zx2c4.com/wireguard/conn"
)

type Outboud struct {
	Endpoint
	RDGetter
	pubkey     string
	wrongCount uint32
	locker     sync.Locker
	ch         chan []byte
}

var _ conn.Endpoint = (*Outboud)(nil)

func NewOutbound(rd RDGetter, pubkey string) *Outboud {
	return &Outboud{
		RDGetter:   rd,
		pubkey:     pubkey,
		wrongCount: 1,
		locker:     &sync.Mutex{},
		ch:         make(chan []byte),
	}
}

type RDGetter interface {
	GetRoutingDiscovery() *drouting.RoutingDiscovery
	GetHost() host.Host
}

func (ep *Outboud) DstToBytes() []byte {
	return []byte(ep.pubkey)
}

func (ep *Outboud) Messages() <-chan []byte {
	return ep.ch
}

var _ Sender = (*Outboud)(nil)

func (ep *Outboud) Send(buf []byte) (err error) {
	defer func() {
		if err != nil {
			atomic.AddUint32(&ep.wrongCount, 1)
		}
	}()
	c := atomic.LoadUint32(&ep.wrongCount)
	if buf[0] == 1 {
		if err := ep.Connect(); err != nil {
			return err
		}
	}
	if c != 0 || ep.Stream == nil {
		return net.ErrClosed
	}
	written, err := io.Copy(ep.Stream, bytes.NewReader(buf))
	_ = written
	return err
}

func (ep *Outboud) Connect() (err error) {
	defer err0.Then(&err, func() {
		atomic.StoreUint32(&ep.wrongCount, 0)
	}, nil)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rd := ep.GetRoutingDiscovery()
	host := ep.GetHost()
	peerCh := try.To1(rd.FindPeers(ctx, ep.pubkey, discovery.Limit(10)))
	for peer := range peerCh {
		if len(peer.Addrs) == 0 {
			continue
		}
		ep.Stream, err = host.NewStream(context.Background(), peer.ID, ProtocolID)
		if err != nil {
			slog.Error("peer连接失败", "peer id", peer.ID, "err", err)
			continue
		}
		go func() {
			for {
				var p [512]byte
				n, err := ep.Stream.Read(p[:])
				if err != nil {
					return
				}
				ep.ch <- p[:n]
			}
		}()
		return nil
	}
	return ErrNoPeerConnected
}

var ErrNoPeerConnected = fmt.Errorf("没有连接到任一节点")
