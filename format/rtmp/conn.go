package rtmp

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/url"
	"sync"

	"github.com/tuan3w/joy5/format/flv/flvio"
)

const (
	//ErrConnectRejected error connect rejected
	ErrConnectRejected = "NetConnection.Connect.Rejected"

	//ErrPublishBadname invalid app name
	ErrPublishBadname = "NetStream.Publish.BadName"

	//ErrConnectAppShutdown service temporary unavailable
	ErrConnectAppShutdown = "NetConnection.Connect.AppShutdown"
)

type ReadWriteFlusher interface {
	io.ReadWriter
	Flush() error
}

type Conn struct {
	LogStageEvent       func(event string, url string)
	LogChunkDataEvent   func(isRead bool, b []byte)
	LogChunkHeaderEvent func(isRead bool, m message)
	LogTagEvent         func(isRead bool, t flvio.Tag)
	LogMsgEvent         func(isRead bool, m message)

	HandleEvent func(msgtypeid uint8, msgdata []byte) (handled bool, err error)

	URL      *url.URL
	PageUrl  string
	TcUrl    string
	FlashVer string

	PubPlayErr            error
	PubPlayOnStatusParams flvio.AMFMap

	closeNotify chan bool

	wrapRW *wrapReadWriter

	peekread chan *message

	avmsgsid            uint32
	writebuf, writebuf2 []byte
	readbuf, readbuf2   []byte
	lastackn, ackn      uint32
	writeMaxChunkSize   int
	ReadMaxChunkSize    int
	readAckSize         uint32
	readcsmap           map[uint32]*message
	aggmsg              *message

	lastcmd *command

	isserver   bool
	Publishing bool
	Stage      Stage

	SendSampleAccess bool
	BypassMsgtypeid  []uint8

	// store error happened to connection
	// will be useful for OnDone method
	Error error

	// flag indicates that livestream is complete
	// normally without this flag is set, client can
	// reconnect to broadcast livestream
	Complete bool

	netConn net.Conn

	l *sync.Mutex
}

func NewConn(nc net.Conn) *Conn {
	rw := &bufReadWriter{
		Reader: bufio.NewReaderSize(nc, BufioSize),
		Writer: bufio.NewWriterSize(nc, BufioSize),
	}
	c := &Conn{}
	c.netConn = nc
	c.closeNotify = make(chan bool, 1)
	c.wrapRW = newWrapReadWriter(c, rw)
	c.readcsmap = make(map[uint32]*message)
	c.ReadMaxChunkSize = 128
	c.writeMaxChunkSize = 128
	c.writebuf = make([]byte, 256)
	c.writebuf2 = make([]byte, 256)
	c.readbuf = make([]byte, 256)
	c.readbuf2 = make([]byte, 256)
	c.readAckSize = 2500000
	c.l = &sync.Mutex{}
	return c
}

func (c *Conn) writing() bool {
	if c.isserver {
		return !c.Publishing
	} else {
		return c.Publishing
	}
}

func (c *Conn) NetConn() net.Conn {
	return c.netConn
}

func (c *Conn) writePubPlayErrBeforeClose() {
	if c.PubPlayErr == nil {
		return
	}
	if c.Stage < StageGotPublishOrPlayCommand {
		return
	}
	c.Prepare(StageCommandDone, PrepareWriting)
}

func (c *Conn) flushWrite() error {
	return c.wrapRW.Flush()
}

func (c *Conn) CloseNotify() <-chan bool {
	return c.closeNotify
}

func (c *Conn) ForceClose(errCode string, msg string) (err error) {
	if errCode == "" {
		errCode = "NetStream.Play.Stop"
	}

	if err = c.writeCommand(5, c.avmsgsid,
		"onStatus", c.lastcmd.transid, nil,
		flvio.AMFMap{
			{K: "level", V: "status"},
			{K: "code", V: errCode},
			{K: "description", V: msg},
		},
	); err != nil {
		return
	}

	c.Error = errors.New(msg)

	c.flushWrite()

	return nil
}

func (c *Conn) Close() {
	c.writeCommand(5, c.avmsgsid, "onStatus", c.lastcmd.transid, nil, flvio.AMFMap{
		{K: "level", V: "status"},
		{K: "code", V: "NetStream.Play.Stop"},
		{K: "description", V: "Stopped stream"},
	})
	c.flushWrite()
}
