package ops

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/sshx"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPOpenFunc opens an SFTP channel on an established SSH client. It is the
// single SFTP acquisition point; production uses defaultSFTPOpen.
type SFTPOpenFunc func(sshx.Client) (SFTPConn, error)

// hostSession is a reusable SSH connection with a lazily-opened SFTP channel.
// It is owned by the sessionManager: individual operations never close it.
type hostSession struct {
	client sshx.Client
	sftp   SFTPConn
	sftpMu sync.Mutex
	broken bool
}

// sessionManager is the single acquisition path for SSH connections. All
// operations (Exec, transfer, Stat, DialTCP) must go through acquire so that
// connection reuse and failure-reconnect are enforced in one place instead of
// being re-implemented at each call site.
type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]*hostSession
	dial     DialFunc
	sftp     SFTPOpenFunc
}

func newSessionManager(dial DialFunc, sftpOpen SFTPOpenFunc) *sessionManager {
	return &sessionManager{
		sessions: map[string]*hostSession{},
		dial:     dial,
		sftp:     sftpOpen,
	}
}

// acquire returns the cached session for the host or dials once and caches it.
// Sessions are keyed by alias, which keeps distinct jump-host routes separate.
func (m *sessionManager) acquire(ctx context.Context, host config.Host, connectTimeout time.Duration) (*hostSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.sessions[host.Alias]; ok {
		if session.broken {
			m.closeLocked(session)
			delete(m.sessions, host.Alias)
		} else {
			return session, nil
		}
	}
	if m.dial == nil {
		return nil, fmt.Errorf("连接拨号器未配置")
	}
	client, err := m.dial(ctx, host, connectTimeout)
	if err != nil {
		return nil, err
	}
	session := &hostSession{client: client}
	m.sessions[host.Alias] = session
	return session, nil
}

// sftpFor lazily opens and returns the session's shared SFTP channel.
func (m *sessionManager) sftpFor(session *hostSession) (SFTPConn, error) {
	session.sftpMu.Lock()
	defer session.sftpMu.Unlock()
	if session.sftp != nil {
		return session.sftp, nil
	}
	if m.sftp == nil {
		return nil, fmt.Errorf("SFTP 打开器未配置")
	}
	conn, err := m.sftp(session.client)
	if err != nil {
		return nil, err
	}
	session.sftp = conn
	return conn, nil
}

// markBroken closes and removes the cached session so the next operation
// re-dials. Failed operations themselves are never retried; this only heals
// the connection for subsequent tasks.
func (m *sessionManager) markBroken(alias string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.sessions[alias]; ok {
		m.closeLocked(session)
		delete(m.sessions, alias)
	}
}

func (m *sessionManager) closeLocked(session *hostSession) {
	if session.sftp != nil {
		_ = session.sftp.Close()
	}
	_ = session.client.Close()
}

// closeAll releases every cached connection.
func (m *sessionManager) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for alias, session := range m.sessions {
		m.closeLocked(session)
		delete(m.sessions, alias)
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
