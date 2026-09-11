package platforminstall

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// ErrMetadataLeaseConflict refuses occupied, changed, missing or expired leases.
var ErrMetadataLeaseConflict = errors.New("metadata lease unavailable or mismatched")

// MetadataLeaseRequest binds one nonrenewable, monotonic-deadline transaction.
type MetadataLeaseRequest struct {
	Transaction   string `json:"transaction" minLength:"64" maxLength:"64" pattern:"^[0-9a-f]{64}$"`
	RequestSHA256 string `json:"request_sha256" minLength:"64" maxLength:"64" pattern:"^[0-9a-f]{64}$"`
	Nonce         string `json:"nonce" minLength:"64" maxLength:"64" pattern:"^[0-9a-f]{64}$"`
	Deadline      int64  `json:"deadline" minimum:"1"`
	Observation   bool   `json:"observation,omitempty"`
}

// MetadataLeaseCheck identifies the exact request and supervisor instance.
type MetadataLeaseCheck struct {
	Request  MetadataLeaseRequest `json:"request"`
	Instance string               `json:"instance" minLength:"64" maxLength:"64" pattern:"^[0-9a-f]{64}$"`
}

// MetadataLeaseProof describes current in-memory lease evidence, never a grant
// that can be replayed from a file or used as permanent runtime authority.
type MetadataLeaseProof struct {
	Request  MetadataLeaseRequest `json:"request"`
	Instance string               `json:"instance"`
	Observed int64                `json:"observed"`
}

// MetadataLease holds one bounded slot for the lifetime of a supervisor image.
type MetadataLease struct {
	mu       sync.Mutex
	instance string
	clock    func() (int64, error)
	request  MetadataLeaseRequest
	active   bool
}

// NewMetadataLease creates a fresh image-local identity without durable state.
func NewMetadataLease() *MetadataLease {
	var nonce [32]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return newMetadataLease("", func() (int64, error) { return 0, err })
	}
	return newMetadataLease(hex.EncodeToString(nonce[:]), metadataMonotonicNow)
}

func newMetadataLease(instance string, clock func() (int64, error)) *MetadataLease {
	return &MetadataLease{instance: instance, clock: clock}
}

func validateMetadataLeaseRequest(request MetadataLeaseRequest, now int64) error {
	for name, value := range map[string]string{"transaction": request.Transaction, "request": request.RequestSHA256, "nonce": request.Nonce} {
		if err := validateSHA256(name, value); err != nil {
			return err
		}
	}
	if now < 0 || request.Deadline <= now || request.Deadline-now > 30_000_000_000 {
		return fmt.Errorf("metadata lease must expire within 30 monotonic seconds")
	}
	return nil
}

// Begin reserves an empty or expired slot; it never renews an active lease.
func (lease *MetadataLease) Begin(request MetadataLeaseRequest) (MetadataLeaseProof, error) {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	now, err := lease.clock()
	if err != nil {
		return MetadataLeaseProof{}, err
	}
	if err := validateSHA256("instance", lease.instance); err != nil {
		return MetadataLeaseProof{}, err
	}
	if err := validateMetadataLeaseRequest(request, now); err != nil {
		return MetadataLeaseProof{}, err
	}
	if lease.active && now < lease.request.Deadline {
		return MetadataLeaseProof{}, ErrMetadataLeaseConflict
	}
	if !request.Observation {
		lease.request, lease.active = request, true
	}
	return MetadataLeaseProof{request, lease.instance, now}, nil
}

// Check observes the same instance and exact unexpired request without renewal.
func (lease *MetadataLease) Check(check MetadataLeaseCheck) (MetadataLeaseProof, error) {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	now, err := lease.clock()
	if err != nil {
		return MetadataLeaseProof{}, err
	}
	if err := validateMetadataLeaseRequest(check.Request, now); err != nil {
		return MetadataLeaseProof{}, ErrMetadataLeaseConflict
	}
	if check.Instance != lease.instance || validateSHA256("instance", lease.instance) != nil {
		return MetadataLeaseProof{}, ErrMetadataLeaseConflict
	}
	if check.Request.Observation {
		if lease.active && now < lease.request.Deadline {
			return MetadataLeaseProof{}, ErrMetadataLeaseConflict
		}
		return MetadataLeaseProof{check.Request, lease.instance, now}, nil
	}
	if !lease.active || check.Request != lease.request || now >= lease.request.Deadline {
		return MetadataLeaseProof{}, ErrMetadataLeaseConflict
	}
	return MetadataLeaseProof{lease.request, lease.instance, now}, nil
}
