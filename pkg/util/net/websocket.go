package net

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
)

var ErrWebsocketListenerClosed = errors.New("websocket listener closed")

const (
	FrpWebsocketPath = "/~!frp"

	websocketMethodPrefix = "GET "
)

// MatchWebsocketFunc returns the number of bytes needed to identify a websocket
// handshake request and a function reporting whether the data starts with such a
// request. If paths is empty, every HTTP GET request matches, whatever its path.
func MatchWebsocketFunc(paths []string) (needBytesNum uint32, matchFn func(data []byte) bool) {
	if len(paths) == 0 {
		prefix := []byte(websocketMethodPrefix)
		return uint32(len(prefix)), func(data []byte) bool {
			return bytes.HasPrefix(data, prefix)
		}
	}

	prefixes := make([][]byte, 0, len(paths))
	for _, path := range paths {
		prefix := []byte(websocketMethodPrefix + path)
		prefixes = append(prefixes, prefix)
		// One more byte than the prefix, so that the byte following the path can be
		// checked and "/frp" doesn't match a request for "/frpx".
		needBytesNum = max(needBytesNum, uint32(len(prefix)+1))
	}
	return needBytesNum, func(data []byte) bool {
		for _, prefix := range prefixes {
			if !bytes.HasPrefix(data, prefix) {
				continue
			}
			// The request target ends here, only a space (before the HTTP version) or
			// the start of a query string may follow.
			if len(data) > len(prefix) && data[len(prefix)] != ' ' && data[len(prefix)] != '?' {
				continue
			}
			return true
		}
		return false
	}
}

type WebsocketListener struct {
	ln       net.Listener
	acceptCh chan net.Conn

	server *http.Server
}

// NewWebsocketListener to handle websocket connections
// ln: tcp listener for websocket connections
// paths: HTTP paths accepted for the websocket handshake. If it's empty, requests
// for any path are accepted.
func NewWebsocketListener(ln net.Listener, paths ...string) (wl *WebsocketListener) {
	wl = &WebsocketListener{
		ln:       ln,
		acceptCh: make(chan net.Conn),
	}

	wsHandler := websocket.Handler(func(c *websocket.Conn) {
		// The tunnel payload is a raw byte stream (yamux), not UTF-8 text.
		// Send it as binary frames; otherwise RFC 6455-compliant intermediaries
		// (e.g. API gateways/reverse proxies) UTF-8-validate the default text
		// frames and close the connection on invalid bytes.
		c.PayloadType = websocket.BinaryFrame
		notifyCh := make(chan struct{})
		conn := WrapCloseNotifyConn(c, func(_ error) {
			close(notifyCh)
		})
		wl.acceptCh <- conn
		<-notifyCh
	})

	allowedPaths := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		allowedPaths[path] = struct{}{}
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(allowedPaths) > 0 {
			if _, ok := allowedPaths[r.URL.Path]; !ok {
				http.NotFound(w, r)
				return
			}
		}
		wsHandler.ServeHTTP(w, r)
	})

	wl.server = &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           handler,
		ReadHeaderTimeout: 60 * time.Second,
	}

	go func() {
		_ = wl.server.Serve(ln)
	}()
	return
}

func (p *WebsocketListener) Accept() (net.Conn, error) {
	c, ok := <-p.acceptCh
	if !ok {
		return nil, ErrWebsocketListenerClosed
	}
	return c, nil
}

func (p *WebsocketListener) Close() error {
	return p.server.Close()
}

func (p *WebsocketListener) Addr() net.Addr {
	return p.ln.Addr()
}
