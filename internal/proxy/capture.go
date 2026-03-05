package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wesaver/internal/models"
)

type Capture struct {
	mu       sync.Mutex
	listener net.Listener
	port     int
	running  bool
	ca       *CA

	directTransport *http.Transport

	candidates     chan string
	candidateDedup sync.Map

	totalRequests   atomic.Int64
	wechatRequests  atomic.Int64
	profileRequests atomic.Int64
	candidateCount  atomic.Int64

	lastRequestAt   atomic.Value
	lastRequestHost atomic.Value
}

func NewCapture(ca *CA) *Capture {
	return &Capture{
		ca:         ca,
		candidates: make(chan string, 100),
		directTransport: &http.Transport{
			Proxy:               nil,
			MaxIdleConns:        32,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

func (c *Capture) Port() int { return c.port }

func (c *Capture) CA() *CA { return c.ca }

func (c *Capture) Start(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("proxy already running")
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	c.listener = ln
	c.port = port
	c.running = true

	go c.serve()
	return nil
}

func (c *Capture) serve() {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}
		go c.handleConnection(conn)
	}
}

func (c *Capture) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	buf := make([]byte, 8192)
	n, err := clientConn.Read(buf)
	if err != nil {
		return
	}

	request := string(buf[:n])
	c.totalRequests.Add(1)
	c.lastRequestAt.Store(time.Now())

	if strings.HasPrefix(request, "CONNECT ") {
		c.handleConnect(clientConn, request)
		return
	}

	// Handle plain HTTP
	firstLine := strings.SplitN(request, "\r\n", 2)[0]
	parts := strings.Fields(firstLine)
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return
	}

	method := parts[0]
	rawURL := parts[1]
	if !strings.Contains(rawURL, "://") {
		hostHeader := extractHostHeader(request)
		if hostHeader == "" {
			return
		}
		if strings.HasPrefix(rawURL, "/") {
			rawURL = "http://" + hostHeader + rawURL
		} else {
			rawURL = "http://" + hostHeader + "/" + rawURL
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := u.Host
	if host == "" {
		host = extractHostFromURL(rawURL)
	}
	c.lastRequestHost.Store(host)

	c.inspectURL(rawURL)

	// Forward using the shared transport that bypasses system proxy to avoid infinite loop
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Connection", "close")
	resp, err := c.directTransport.RoundTrip(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_ = resp.Write(clientConn)
}

func (c *Capture) handleConnect(clientConn net.Conn, request string) {
	firstLine := strings.SplitN(request, "\r\n", 2)[0]
	parts := strings.Fields(firstLine)
	if len(parts) < 2 {
		return
	}

	host := normalizeConnectHost(parts[1])
	if host == "" {
		return
	}
	c.lastRequestHost.Store(host)

	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil && h != "" {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	isWechatMP := strings.EqualFold(hostname, "mp.weixin.qq.com")

	// Send 200 Connection Established
	if err := writeFull(clientConn, []byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	if !isWechatMP {
		// For non-wechat, just tunnel
		targetConn, err := net.DialTimeout("tcp", host, 10*time.Second)
		if err != nil {
			return
		}
		defer targetConn.Close()

		done := make(chan struct{})
		go func() {
			io.Copy(targetConn, clientConn)
			close(done)
		}()
		io.Copy(clientConn, targetConn)
		<-done
		return
	}

	c.wechatRequests.Add(1)

	// For WeChat: MITM with TLS to inspect URLs
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	targetConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", host, tlsConfig)
	if err != nil {
		return
	}
	defer targetConn.Close()

	// Sign a leaf cert for this host using our CA
	cert, err := c.ca.SignHost(hostname)
	if err != nil {
		return
	}

	tlsClientConn := tls.Server(clientConn, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err := tlsClientConn.Handshake(); err != nil {
		return
	}

	// Read the actual HTTPS request
	buf := make([]byte, 32768)
	n, err := tlsClientConn.Read(buf)
	if err != nil {
		return
	}

	// Inspect the URL from the decrypted request
	reqFirstLine := strings.SplitN(string(buf[:n]), "\r\n", 2)[0]
	reqParts := strings.Fields(reqFirstLine)
	if len(reqParts) >= 2 {
		path := reqParts[1]
		fullURL := buildHTTPSURL(hostname, path)
		c.inspectURL(fullURL)

		if strings.Contains(fullURL, "/mp/profile_ext") {
			c.profileRequests.Add(1)
		}
	}

	// Forward to target
	if err := writeFull(targetConn, buf[:n]); err != nil {
		return
	}

	// Relay back
	done := make(chan struct{})
	go func() {
		io.Copy(targetConn, tlsClientConn)
		close(done)
	}()
	io.Copy(tlsClientConn, targetConn)
	<-done
}

var requiredFragments = []string{"__biz=", "uin=", "key=", "pass_ticket="}

func (c *Capture) inspectURL(rawURL string) {
	if rawURL == "" {
		return
	}

	lower := strings.ToLower(rawURL)
	if !strings.Contains(lower, "weixin.qq.com") {
		return
	}

	for _, frag := range requiredFragments {
		if !strings.Contains(lower, strings.ToLower(frag)) {
			return
		}
	}

	if _, loaded := c.candidateDedup.LoadOrStore(rawURL, true); !loaded {
		if _, ok := models.ParseTokenLink(rawURL); ok {
			select {
			case c.candidates <- rawURL:
				c.candidateCount.Add(1)
			default:
			}
		}
	}
}

func (c *Capture) TryGetCapturedToken() (string, bool) {
	select {
	case url := <-c.candidates:
		return url, true
	default:
		return "", false
	}
}

func (c *Capture) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.listener != nil {
		c.listener.Close()
	}
	if c.directTransport != nil {
		c.directTransport.CloseIdleConnections()
	}
	c.running = false
}

func (c *Capture) GetStats() map[string]interface{} {
	c.mu.Lock()
	running := c.running
	port := c.port
	c.mu.Unlock()

	lastAt := ""
	if v := c.lastRequestAt.Load(); v != nil {
		if t, ok := v.(time.Time); ok {
			lastAt = t.Format("15:04:05")
		}
	}

	lastHost := ""
	if v := c.lastRequestHost.Load(); v != nil {
		if s, ok := v.(string); ok {
			lastHost = s
		}
	}

	return map[string]interface{}{
		"total_requests":   c.totalRequests.Load(),
		"wechat_requests":  c.wechatRequests.Load(),
		"profile_requests": c.profileRequests.Load(),
		"candidate_count":  c.candidateCount.Load(),
		"last_request_at":  lastAt,
		"last_request_host": lastHost,
		"running":          running,
		"port":             port,
	}
}

func extractHostFromURL(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rest := rawURL[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
			return rest[:slashIdx]
		}
		return rest
	}
	if slashIdx := strings.Index(rawURL, "/"); slashIdx >= 0 {
		return rawURL[:slashIdx]
	}
	return rawURL
}

func extractHostHeader(rawRequest string) string {
	for _, line := range strings.Split(rawRequest, "\r\n") {
		if !strings.HasPrefix(strings.ToLower(line), "host:") {
			continue
		}
		return strings.TrimSpace(line[5:])
	}
	return ""
}

func normalizeConnectHost(rawHost string) string {
	host := strings.TrimSpace(rawHost)
	if host == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return host + ":443"
	}
	return net.JoinHostPort(host, "443")
}

func buildHTTPSURL(hostname, path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return "https://" + hostname + "/"
	}
	lp := strings.ToLower(p)
	if strings.HasPrefix(lp, "https://") || strings.HasPrefix(lp, "http://") {
		return p
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "https://" + hostname + p
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
