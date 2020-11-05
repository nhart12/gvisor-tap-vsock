package virtualnetwork

import (
	"net"
	"net/http"

	"github.com/code-ready/gvisor-tap-vsock/pkg/services/dns"
	"github.com/code-ready/gvisor-tap-vsock/pkg/services/forwarder"
	"github.com/code-ready/gvisor-tap-vsock/pkg/types"
	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

func addServices(configuration *types.Configuration, s *stack.Stack) (http.Handler, error) {
	tcpForwarder := forwarder.TCP(s)
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpForwarder.HandlePacket)
	udpForwarder := forwarder.UDP(s)
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpForwarder.HandlePacket)

	if err := dnsServer(configuration, s); err != nil {
		return nil, err
	}

	forwarderMux, err := forwardHostVM(configuration, s)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/forwarder/", http.StripPrefix("/forwarder", forwarderMux))
	return mux, nil
}

func dnsServer(configuration *types.Configuration, s *stack.Stack) error {
	udpConn, err := gonet.DialUDP(s, &tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.Address(net.ParseIP(configuration.GatewayIP).To4()),
		Port: uint16(53),
	}, nil, ipv4.ProtocolNumber)
	if err != nil {
		return err
	}

	go func() {
		if err := dns.Serve(udpConn, configuration.DNS); err != nil {
			log.Error(err)
		}
	}()
	return nil
}

func forwardHostVM(configuration *types.Configuration, s *stack.Stack) (http.Handler, error) {
	fw := forwarder.NewPortsForwarder(s)
	for local, remote := range configuration.Forwards {
		if err := fw.Expose(local, remote); err != nil {
			return nil, err
		}
	}
	return fw.Mux(), nil
}
