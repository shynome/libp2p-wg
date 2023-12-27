package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/shynome/err0/try"
	bind "github.com/shynome/wg-libp2p-bind"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type peer struct {
	key string
	ip  string
}

var keys = []peer{
	{"aPU2wWTztz35vRbB/+EvBTXMiqAfUW9oQ7J1aoy8gnk=", "192.168.5.1/32"},
	{"EIKKCxnbnIcgSh7rnqjlsJy/1z/L8G6sZzGPjdcrJnE=", "192.168.5.2/32"},
	// {"8GFcYBs2b4W7cZeCnXDrRNdrzSCF6ODzfK6aRAwX1Vc=", "192.168.5.3/32"},
}

var args struct {
	key         int
	autoConnect bool
}

func init() {
	flag.IntVar(&args.key, "key", -1, "选择指定")
	flag.BoolVar(&args.autoConnect, "c", false, "自动连接节点")
}

func main() {
	flag.Parse()
	if args.key < 0 {
		slog.Error("选择要使用的密钥")
		return
	}

	user := try.To1(user.Current())
	rootUser := user.Name == "root"
	peer := keys[args.key]

	var tdev tun.Device
	if !rootUser {
		ip := strings.ReplaceAll(peer.ip, "/32", "")
		tdev, _ = try.To2(netstack.CreateNetTUN(
			[]netip.Addr{netip.MustParseAddr(ip)},
			nil,
			1800,
		))
	} else {
		tdev = try.To1(tun.CreateTUN("wg2", 1800))
	}

	logger := device.NewLogger(
		device.LogLevelVerbose,
		fmt.Sprintf("peer (%d) ", args.key),
	)

	bind := bind.New()
	{
		rawKey := try.To1(base64.StdEncoding.DecodeString(peer.key))
		key := try.To1(wgtypes.NewKey(rawKey))
		pubkey := key.PublicKey()
		bind.Pubkey = hex.EncodeToString(pubkey[:])
	}

	dev := device.NewDevice(tdev, bind, logger)

	conf := genConf(args.key)
	try.To(dev.IpcSet(conf))

	if rootUser {
		ifname := try.To1(tdev.Name())
		{
			route := strings.ReplaceAll(peer.ip, "32", "24")
			cmd := exec.Command("ip", "addr", "add", route, "dev", ifname)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			try.To(cmd.Run())
		}
		{
			cmd := exec.Command("ip", "link", "set", ifname, "up")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			try.To(cmd.Run())
		}

		fileUAPI := try.To1(ipc.UAPIOpen(ifname))
		uapi := try.To1(ipc.UAPIListen(ifname, fileUAPI))
		go func() {
			for {
				conn := try.To1(uapi.Accept())
				go dev.IpcHandle(conn)
			}
		}()
	} else {
		err := dev.Up()
		try.To(err)
	}

	<-dev.Wait()
}

func genConf(index int) string {
	buf := bytes.NewBufferString("")

	{
		peer := keys[index]
		key := try.To1(base64.StdEncoding.DecodeString(peer.key))
		fmt.Fprintf(buf, "private_key=%s\n", hex.EncodeToString(key))
	}

	for i, peer := range keys {
		if i == index {
			continue
		}
		rawKey := try.To1(base64.StdEncoding.DecodeString(peer.key))
		key := try.To1(wgtypes.NewKey(rawKey))
		pubkey := key.PublicKey()
		fmt.Fprintf(buf, "public_key=%s\n", hex.EncodeToString(pubkey[:]))
		fmt.Fprintf(buf, "allowed_ip=%s\n", peer.ip)
		fmt.Fprintf(buf, "endpoint=%s\n", hex.EncodeToString(pubkey[:]))
		if args.autoConnect {
			fmt.Fprintf(buf, "persistent_keepalive_interval=%d\n", 15)
		}
	}

	return buf.String()
}
