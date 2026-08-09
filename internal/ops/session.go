package ops

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/sshx"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPOpenFunc opens an SFTP channel on an established SSH client. It is the
// single SFTP acquisition point; production uses defaultSFTPOpen.
type SFTPOpenFunc func(sshx.Client) (SFTPConn, error)

var errSessionInvalidated = errors.New("SSH 会话已关闭或失效")

// hostSession is a reusable SSH connection with a lazily-opened SFTP channel.
// It is owned by the sessionManager: individual operations never close it.
type hostSession struct {
	client sshx.Client

	operationGate chan struct{}
	sftpMu        sync.Mutex
	sftp          SFTPConn
	sftpCall      *sftpOpenCall
	closed        bool
	closeOnce     sync.Once
}

type sftpOpenCall struct {
	done chan struct{}
	conn SFTPConn
	err  error
}

type dialCall struct {
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   int
	delivered bool
	session   *hostSession
	err       error
}

// sessionManager is the single acquisition path for SSH connections. The
// manager mutex only protects maps; network dials and connection shutdown are
// deliberately performed without it so unrelated hosts remain independent.
type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]*hostSession
	inflight map[string]*dialCall
	dial     DialFunc
	sftp     SFTPOpenFunc
}

func newSessionManager(dial DialFunc, sftpOpen SFTPOpenFunc) *sessionManager {
	return &sessionManager{
		sessions: map[string]*hostSession{},
		inflight: map[string]*dialCall{},
		dial:     dial,
		sftp:     sftpOpen,
	}
}

// acquire returns the cached session for a host. Concurrent callers for the
// same host share one in-flight dial, while different hosts dial in parallel.
func (m *sessionManager) acquire(ctx context.Context, host config.Host, connectTimeout time.Duration) (*hostSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	if session := m.sessions[host.Alias]; session != nil {
		m.mu.Unlock()
		return session, nil
	}
	call := m.inflight[host.Alias]
	if call == nil {
		if m.dial == nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("连接拨号器未配置")
		}
		// A shared dial must not inherit the first waiter's cancellation: another
		// caller for the same host may still be waiting. The dial is canceled as
		// soon as its last waiter leaves, while DialFunc enforces connectTimeout.
		dialCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		call = &dialCall{done: make(chan struct{}), cancel: cancel, waiters: 1}
		m.inflight[host.Alias] = call
		go m.runDial(dialCtx, host, connectTimeout, call)
	} else {
		call.waiters++
	}
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		m.releaseDialWaiter(host.Alias, call, true)
		return nil, ctx.Err()
	case <-call.done:
		if err := ctx.Err(); err != nil {
			m.releaseDialWaiter(host.Alias, call, true)
			return nil, err
		}
		m.releaseDialWaiter(host.Alias, call, false)
		if call.err != nil {
			return nil, call.err
		}
		m.mu.Lock()
		current := m.sessions[host.Alias] == call.session
		m.mu.Unlock()
		if !current {
			return nil, errSessionInvalidated
		}
		return call.session, nil
	}
}

// beginOperation serializes operations for one host while preserving full
// concurrency across hosts. This makes cancellation ownership unambiguous: an
// active SFTP operation may close its session without interrupting another
// valid operation on the same host, while canceled waiters simply return.
func (m *sessionManager) beginOperation(ctx context.Context, host config.Host, connectTimeout time.Duration) (*hostSession, func(), error) {
	for {
		session, err := m.acquire(ctx, host, connectTimeout)
		if err != nil {
			return nil, nil, err
		}
		if err := session.beginOperation(ctx); err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			continue
		}
		m.mu.Lock()
		current := m.sessions[host.Alias] == session
		m.mu.Unlock()
		if current {
			return session, session.endOperation, nil
		}
		session.endOperation()
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
	}
}

func (s *hostSession) beginOperation(ctx context.Context) error {
	select {
	case s.operationGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.sftpMu.Lock()
	closed := s.closed
	s.sftpMu.Unlock()
	if closed {
		<-s.operationGate
		return errSessionInvalidated
	}
	return nil
}

func (s *hostSession) endOperation() {
	<-s.operationGate
}

func (m *sessionManager) releaseDialWaiter(alias string, call *dialCall, canceled bool) {
	m.mu.Lock()
	if call.waiters > 0 {
		call.waiters--
	}
	if !canceled && call.err == nil && call.session != nil {
		call.delivered = true
	}
	current := m.inflight[alias] == call
	shouldCancel := canceled && call.waiters == 0 && current
	shouldInvalidate := canceled && call.waiters == 0 && !current && call.session != nil && !call.delivered
	if shouldCancel {
		delete(m.inflight, alias)
	}
	session := call.session
	m.mu.Unlock()

	if shouldCancel {
		call.cancel()
	}
	if shouldInvalidate {
		m.invalidate(alias, session)
	}
}

func (m *sessionManager) runDial(ctx context.Context, host config.Host, connectTimeout time.Duration, call *dialCall) {
	client, err := m.dial(ctx, host, connectTimeout)
	if err == nil && client == nil {
		err = errors.New("连接拨号器返回了空会话")
	}
	var session *hostSession
	if err == nil {
		session = &hostSession{client: client, operationGate: make(chan struct{}, 1)}
	}

	m.mu.Lock()
	current := m.inflight[host.Alias] == call
	if current {
		delete(m.inflight, host.Alias)
	}
	if current && err == nil && ctx.Err() == nil {
		m.sessions[host.Alias] = session
		call.session = session
	} else {
		if err == nil {
			err = errSessionInvalidated
		}
		call.err = err
	}
	if call.err == nil && call.session == nil {
		call.err = err
	}
	close(call.done)
	m.mu.Unlock()
	call.cancel()

	if session != nil && call.session == nil {
		session.close()
	}
}

// sftpFor lazily opens and returns the session's shared SFTP channel. Opening
// is also single-flight, and a canceled waiter invalidates the corresponding
// SSH session so a blocked SFTP handshake is interrupted.
func (m *sessionManager) sftpFor(ctx context.Context, alias string, session *hostSession) (SFTPConn, error) {
	if err := ctx.Err(); err != nil {
		m.invalidate(alias, session)
		return nil, err
	}

	session.sftpMu.Lock()
	if session.closed {
		session.sftpMu.Unlock()
		return nil, errSessionInvalidated
	}
	if session.sftp != nil {
		conn := session.sftp
		session.sftpMu.Unlock()
		return conn, nil
	}
	call := session.sftpCall
	if call == nil {
		if m.sftp == nil {
			session.sftpMu.Unlock()
			return nil, fmt.Errorf("SFTP 打开器未配置")
		}
		call = &sftpOpenCall{done: make(chan struct{})}
		session.sftpCall = call
		go m.runSFTPOpen(session, call)
	}
	session.sftpMu.Unlock()

	select {
	case <-ctx.Done():
		m.invalidate(alias, session)
		return nil, ctx.Err()
	case <-call.done:
		if err := ctx.Err(); err != nil {
			m.invalidate(alias, session)
			return nil, err
		}
		if call.err != nil {
			m.invalidate(alias, session)
			return nil, call.err
		}
		return call.conn, call.err
	}
}

func (m *sessionManager) runSFTPOpen(session *hostSession, call *sftpOpenCall) {
	conn, err := m.sftp(session.client)
	if err == nil && conn == nil {
		err = errors.New("SFTP 打开器返回了空连接")
	}

	session.sftpMu.Lock()
	closed := session.closed
	if !closed && err == nil {
		session.sftp = conn
		call.conn = conn
	} else {
		if err == nil {
			err = errSessionInvalidated
		}
		call.err = err
	}
	if call.err == nil && call.conn == nil {
		call.err = err
	}
	if session.sftpCall == call {
		session.sftpCall = nil
	}
	close(call.done)
	session.sftpMu.Unlock()

	if closed && conn != nil {
		_ = conn.Close()
	}
}

// invalidate removes session only if it is still the cached generation for
// alias. A late cancellation therefore cannot close a replacement session.
func (m *sessionManager) invalidate(alias string, expected *hostSession) {
	m.mu.Lock()
	session := m.sessions[alias]
	if session == expected {
		delete(m.sessions, alias)
	} else {
		session = nil
	}
	m.mu.Unlock()
	if session != nil {
		session.close()
	}
}

func (s *hostSession) close() {
	s.closeOnce.Do(func() {
		s.sftpMu.Lock()
		s.closed = true
		conn := s.sftp
		s.sftp = nil
		s.sftpMu.Unlock()
		_ = s.client.Close()
		if conn != nil {
			_ = conn.Close()
		}
	})
}

// closeAll releases every cached connection and cancels in-flight dials. New
// calls may still create fresh sessions if the executor is used again.
func (m *sessionManager) closeAll() {
	m.mu.Lock()
	sessions := make([]*hostSession, 0, len(m.sessions))
	for alias, session := range m.sessions {
		sessions = append(sessions, session)
		delete(m.sessions, alias)
	}
	calls := make([]*dialCall, 0, len(m.inflight))
	for alias, call := range m.inflight {
		calls = append(calls, call)
		delete(m.inflight, alias)
	}
	m.mu.Unlock()

	for _, call := range calls {
		call.cancel()
	}
	for _, session := range sessions {
		session.close()
	}
}

// defaultSFTPOpen opens a real SFTP channel from a concrete *ssh.Client.
func defaultSFTPOpen(client sshx.Client) (SFTPConn, error) {
	sshClient, ok := client.(*ssh.Client)
	if !ok {
		return nil, fmt.Errorf("会话不支持 SFTP 复用")
	}
	return sftp.NewClient(sshClient)
}
