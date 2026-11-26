package service

import (
	"io"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"

	dnstap "github.com/dnstap/golang-dnstap"
	framestream "github.com/farsightsec/golang-framestream"
	"google.golang.org/protobuf/proto"
)

type DNSTapService struct {
	dnsService *DNSService
	dnsDomain  string
	listener   net.Listener
	mu         sync.Mutex
	running    bool
}

var (
	dnsIndexPrefix    = regexp.MustCompile(`^\d+$`)
	dnsEmbeddedTestID = regexp.MustCompile(`[0-9a-f]{32}`)
)

func NewDNSTapService(dnsDomain string, dnsService *DNSService) *DNSTapService {
	return &DNSTapService{dnsService: dnsService, dnsDomain: dnsDomain}
}

func (s *DNSTapService) Start(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = listener
	s.running = true
	s.mu.Unlock()

	log.Printf("[DNSTap] Listening on %s", addr)
	go s.acceptConnections()
	return nil
}

func (s *DNSTapService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *DNSTapService) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			running := s.running
			s.mu.Unlock()
			if !running {
				return
			}
			log.Printf("[DNSTap] Accept error: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *DNSTapService) handleConnection(conn net.Conn) {
	defer conn.Close()

	decoder, err := framestream.NewDecoder(conn, &framestream.DecoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
	})
	if err != nil {
		log.Printf("[DNSTap] Decoder error: %v", err)
		return
	}

	for {
		data, err := decoder.Decode()
		if err != nil {
			if err != io.EOF {
				log.Printf("[DNSTap] Read frame error: %v", err)
			}
			return
		}
		frame := make([]byte, len(data))
		copy(frame, data)
		s.processFrame(frame)
	}
}

func (s *DNSTapService) processFrame(frame []byte) {
	dt := &dnstap.Dnstap{}
	if err := proto.Unmarshal(frame, dt); err != nil {
		return
	}
	msg := dt.GetMessage()
	if msg == nil {
		return
	}

	msgType := msg.GetType()
	if msgType != dnstap.Message_CLIENT_QUERY && msgType != dnstap.Message_AUTH_QUERY {
		return
	}

	resolverIP := net.IP(msg.GetQueryAddress()).String()
	if resolverIP == "" || resolverIP == "<nil>" {
		return
	}

	queryName := extractQueryName(msg.GetQueryMessage())
	if queryName == "" {
		return
	}

	testID := s.extractTestID(queryName)
	if testID == "" {
		return
	}

	if err := s.dnsService.RecordDNSQuery(testID, resolverIP); err != nil {
		return
	}
	log.Printf("[DNSTap] Recorded testID=%s resolver=%s", testID, resolverIP)
}

func extractQueryName(msg []byte) string {
	if len(msg) < 12 {
		return ""
	}
	offset := 12
	var name strings.Builder
	for offset < len(msg) {
		labelLen := int(msg[offset])
		if labelLen == 0 {
			break
		}
		if labelLen > 63 || offset+1+labelLen > len(msg) {
			return ""
		}
		if name.Len() > 0 {
			name.WriteByte('.')
		}
		name.Write(msg[offset+1 : offset+1+labelLen])
		offset += 1 + labelLen
	}
	return strings.ToLower(name.String())
}

func (s *DNSTapService) extractTestID(queryName string) string {
	domain := strings.ToLower(s.dnsDomain)
	queryName = strings.ToLower(strings.TrimSuffix(queryName, "."))
	if !strings.HasSuffix(queryName, domain) {
		return ""
	}

	prefix := strings.TrimSuffix(queryName, "."+domain)
	if prefix == queryName {
		prefix = strings.TrimSuffix(queryName, domain)
	}

	parts := strings.Split(prefix, ".")
	for _, part := range parts {
		if id := dnsEmbeddedTestID.FindString(part); id != "" {
			return id
		}
	}
	if len(parts) >= 2 && dnsIndexPrefix.MatchString(parts[0]) {
		return parts[1]
	}
	if len(parts) >= 1 && parts[0] != "" {
		return parts[0]
	}
	return ""
}
