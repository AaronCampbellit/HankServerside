package cloud

import (
	"context"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type wsPeer struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func newWSPeer(conn *websocket.Conn) *wsPeer {
	return &wsPeer{conn: conn}
}

func (p *wsPeer) Write(ctx context.Context, payload any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return wsjson.Write(ctx, p.conn, payload)
}
