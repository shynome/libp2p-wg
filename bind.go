package bind

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"github.com/shynome/wg-libp2p-bind/endpoint"
	"golang.zx2c4.com/wireguard/conn"
)

type Bind struct {
	msgCh  chan packetMsg
	close  context.CancelFunc
	rd     *drouting.RoutingDiscovery
	host   host.Host
	Pubkey string
}

var _ conn.Bind = (*Bind)(nil)

func New() *Bind {
	return &Bind{}
}

var bootstrapPeers = func() (infoList []*peer.AddrInfo) {
	ss := []string{
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
		"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
		"/ip4/104.131.131.82/udp/4001/quic/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	}
	for _, s := range ss {
		info := try.To1(peer.AddrInfoFromString(s))
		infoList = append(infoList, info)
	}
	return infoList
}()

func (b *Bind) Open(port uint16) (fns []conn.ReceiveFunc, actualPort uint16, err error) {
	defer err0.Then(&err, nil, nil)

	if b.Pubkey == "" {
		return nil, 0, ErrBindNoPubkey
	}

	ctx := context.Background()
	ctx, b.close = context.WithCancel(ctx)

	b.msgCh = make(chan packetMsg, b.BatchSize()-1)
	fns = append(fns, b.receiveFunc)
	go func() {
		<-ctx.Done()
		close(b.msgCh)
	}()

	ip6quic := fmt.Sprintf("/ip6/::/udp/%d/quic", port)
	ip4quic := fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", port)

	ip6tcp := fmt.Sprintf("/ip6/::/tcp/%d", port)
	ip4tcp := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)

	b.host = try.To1(libp2p.New(
		libp2p.ListenAddrStrings(ip6quic, ip4quic, ip6tcp, ip4tcp),
		libp2p.NATPortMap(),
		libp2p.DefaultMuxers,
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.FallbackDefaults,
	))

	b.host.SetStreamHandler(endpoint.ProtocolID, func(s network.Stream) {
		ep := endpoint.New(s)
		go func() {
			defer ep.Close()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					var p [512]byte
					if n, err := ep.Read(p[:]); err != nil {
						slog.Error("inbound 读取数据失败", "id", ep.ID(), "err", err)
						return
					} else {
						b.msgCh <- packetMsg{
							data: p[:n],
							ep:   ep,
						}
					}
				}
			}
		}()
	})

	kademliaDHT := try.To1(dht.New(ctx, b.host))
	try.To(kademliaDHT.Bootstrap(ctx))

	var wg sync.WaitGroup
	var connectedCount uint32 = 0
	for _, peerInfo := range bootstrapPeers {
		wg.Add(1)
		go func(raddr peer.AddrInfo) {
			defer wg.Done()
			if err := b.host.Connect(ctx, raddr); err != nil {
				slog.Warn("连接失败", "err", err)
				return
			}
			slog.Info("连接成功")
			atomic.AddUint32(&connectedCount, 1)
		}(*peerInfo)
	}
	wg.Wait()

	if c := atomic.LoadUint32(&connectedCount); c == 0 {
		return nil, 0, fmt.Errorf("连接 dht 网络失败")
	}

	b.rd = drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, b.rd, b.Pubkey)
	slog.Warn("通告路由", "pubkey", b.Pubkey)

	return
}

type packetMsg struct {
	data []byte
	ep   conn.Endpoint
}

func (b *Bind) receiveFunc(packets [][]byte, sizes []int, eps []conn.Endpoint) (n int, err error) {
	for i := 0; i < b.BatchSize(); i++ {
		msg, ok := <-b.msgCh
		if !ok {
			return 0, net.ErrClosed
		}
		sizes[i] = copy(packets[i], msg.data)
		eps[i] = msg.ep
		n += 1
	}
	return
}

func (b *Bind) Close() error {
	if b.close != nil {
		b.close()
	}
	return nil
}

var _ endpoint.RDGetter = (*Bind)(nil)

func (b *Bind) GetRoutingDiscovery() *drouting.RoutingDiscovery {
	return b.rd
}

func (b *Bind) GetHost() host.Host {
	return b.host
}

func (b *Bind) ParseEndpoint(pubkey string) (conn.Endpoint, error) {
	outbound := endpoint.NewOutbound(b, pubkey)
	go func() {
		ch := outbound.Messages()
		for d := range ch {
			b.msgCh <- packetMsg{
				data: d,
				ep:   outbound,
			}
		}
	}()
	return outbound, nil
}

func (b *Bind) Send(bufs [][]byte, ep conn.Endpoint) error {
	sender, ok := ep.(endpoint.Sender)
	if !ok {
		return ErrEndpointImpl
	}
	for _, buf := range bufs {
		if err := sender.Send(buf); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bind) SetMark(mark uint32) error { return nil }
func (b *Bind) BatchSize() int            { return 1 }

var ErrEndpointImpl = errors.New("endpoint is not wgortc.Endpoint")
var ErrBindNoPubkey = errors.New("记得为 libp2p bind 设置Pubkey")
