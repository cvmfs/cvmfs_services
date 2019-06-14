package backend

import (
	"context"
	"io"

	gw "github.com/cvmfs/gateway/internal/gateway"
	"github.com/cvmfs/gateway/internal/gateway/receiver"
	"github.com/pkg/errors"
)

// Services is a container for the various
// backend services
type Services struct {
	Access AccessConfig
	Leases LeaseDB
	Pool   *receiver.Pool
	Config gw.Config
}

// ActionController contains the various actions that can be performed with the backend
type ActionController interface {
	GetKey(ctx context.Context, keyID string) *KeyConfig
	GetRepo(ctx context.Context, repoName string) *RepositoryConfig
	GetRepos(ctx context.Context) map[string]RepositoryConfig
	SetRepoEnabled(ctx context.Context, repository string, enabled bool) error
	NewLease(ctx context.Context, keyID, leasePath string, protocolVersion int) (string, error)
	GetLeases(ctx context.Context) (map[string]LeaseReturn, error)
	GetLease(ctx context.Context, tokenStr string) (*LeaseReturn, error)
	CancelLeases(ctx context.Context, repoPath string) error
	CancelLease(ctx context.Context, tokenStr string) error
	CommitLease(ctx context.Context, tokenStr, oldRootHash, newRootHash string, tag gw.RepositoryTag) error
	SubmitPayload(ctx context.Context, token string, payload io.Reader, digest string, headerSize int) error
	RunGC(ctx context.Context, options GCOptions) error
}

// GetKey returns the key configuration associated with a key ID
func (s *Services) GetKey(ctx context.Context, keyID string) *KeyConfig {
	return s.Access.GetKeyConfig(keyID)
}

// StartBackend initializes the various backend services
func StartBackend(cfg *gw.Config) (*Services, error) {
	ac, err := NewAccessConfig(cfg.AccessConfigFile)
	if err != nil {
		return nil, errors.Wrap(
			err, "loading repository access configuration failed")
	}

	ldb, err := OpenLeaseDB(cfg.LeaseDB, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "could not create lease DB")
	}

	pool, err := receiver.StartPool(cfg.ReceiverPath, cfg.NumReceivers, cfg.MockReceiver)
	if err != nil {
		return nil, errors.Wrap(err, "could not start receiver pool")
	}

	return &Services{Access: *ac, Leases: ldb, Pool: pool, Config: *cfg}, nil
}

// Stop all the backend services
func (s *Services) Stop() error {
	if err := s.Leases.Close(); err != nil {
		return errors.Wrap(err, "could not close lease database")
	}
	return nil
}
