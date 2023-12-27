package bind

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/shynome/err0/try"
)

var key1 = try.To1(base64.StdEncoding.DecodeString("UGFCJ4mhmkwrZcIoSwgcHwOFwPQufbzJpDNCshR+D3A="))
var key2 = try.To1(base64.StdEncoding.DecodeString("2DdDuPLoCK6eUXfy1l3nyv4jkK3EtBzT63rCz/FRe0E="))

func TestBind(t *testing.T) {
	bind := New()
	bind.Pubkey = hex.EncodeToString(key1)

	try.To2(bind.Open(6666))
	t.Log(bind)
}

func TestP2P(t *testing.T) {

	host1 := try.To1(libp2p.New())
	cid1 := "0856df2c448a7123d823847c8a674597a3359ff583fe5d635fc56d3c0b67bf75"
	rd := dispatch(host1, cid1)

	host2 := try.To1(libp2p.New())
	cid2 := "b0bfe64894f91269c184b5fe7b65db2166224246f0bc44bf471f0b814bbc8e57"
	dispatch(host2, cid2)

	ctx := context.Background()
	ch := try.To1(rd.FindPeers(ctx, cid2))

	peers := []peer.AddrInfo{}
	for peer := range ch {
		peers = append(peers, peer)
	}

	t.Log("xx", "peers", peers)
}

func dispatch(host host.Host, cid string) *drouting.RoutingDiscovery {
	pid := protocol.ID("test")
	host.SetStreamHandler(pid, func(s network.Stream) {
		slog.Info("ss", "s", s)
	})
	ctx := context.Background()

	kademliaDHT := try.To1(dht.New(ctx, host))
	try.To(kademliaDHT.Bootstrap(ctx))

	var c uint32
	var wg sync.WaitGroup
	for _, info := range bootstrapPeers {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			if err := host.Connect(ctx, pi); err != nil {
				slog.Error("连接失败", "peer id", pi.ID, "err", err)
			} else {
				slog.Info("连接成功")
				atomic.AddUint32(&c, 1)
			}
		}(*info)
	}
	wg.Wait()

	if atomic.LoadUint32(&c) == 0 {
		try.To(fmt.Errorf("连接失败"))
	}

	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, cid)
	slog.Warn("通告路由", "id", cid)

	return routingDiscovery
}
