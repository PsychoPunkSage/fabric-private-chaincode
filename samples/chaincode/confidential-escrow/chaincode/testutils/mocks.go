package testutils

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
)

// MockStub provides a test implementation of MockStubInterface
// It simulates the Fabric ledger state for unit testing without requiring
// a running blockchain network.
type MockStub struct {
	State        map[string][]byte // State stores key-value pairs simulating the ledger
	TransientMap map[string][]byte // TransientMap stores transient data for the current transaction
	TxID         string            // TxID is the simulated transaction ID
	ChannelID    string            // ChannelID is the simulated channel name
	Creator      []byte            // Creator simulates the transaction creator's certificate
	Invocations  []string          // Invocations tracks function calls for verification
}

// NewMockStub creates a new mock stub with initialized state
func NewMockStub() *MockStub {
	mockCert := `-----BEGIN CERTIFICATE-----
MIICGjCCAcCgAwIBAgIRAIQkbh9nsGnLmDalAVlj8sUwCgYIKoZIzj0EAwIwczEL
MAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG
cmFuY2lzY28xGTAXBgNVBAoTEG9yZzEuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh
Lm9yZzEuZXhhbXBsZS5jb20wHhcNMjEwMTAxMDAwMDAwWhcNMzEwMTAxMDAwMDAw
WjBbMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN
U2FuIEZyYW5jaXNjbzEfMB0GA1UEAwwWVXNlcjFAb3JnMS5leGFtcGxlLmNvbTBZ
MBMGByqGSM49AgEGCCqGSM49AwEHA0IABNDopAa0BX6z/jt0+Bm6FEr4VFRj0Lit
jHu6I0v4QjLPqUKLLkqUh9TV6qUvEkYzPLfqkBgPKkLvMjLlH+wkH2+jTTBLMA4G
A1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAAMCsGA1UdIwQkMCKAIKfUfvpKXF7Y
GYCfKLLqJBqhVvYlgaGQHkZQ8gFaZmB8MAoGCCqGSM49BAMCA0gAMEUCIQCxiLcW
wGKvPpLYZxFWCXqjVgslxkXnAZf3JVFC8tQ7NAIgXx3LxA3r4+7VRaMjGhRZQzVl
5lwVdQPjQoaFnhDWGe0=
-----END CERTIFICATE-----`
	// Create a mock serialized identity that cc-tools can parse
	mockIdentity := &msp.SerializedIdentity{
		Mspid:   "Org1MSP", // Match the Callers in your transactions
		IdBytes: []byte(mockCert),
	}

	creatorBytes, _ := proto.Marshal(mockIdentity)

	return &MockStub{
		State:        make(map[string][]byte),
		TransientMap: make(map[string][]byte),
		TxID:         "mock-tx-id",
		ChannelID:    "mock-channel",
		Creator:      creatorBytes,
		Invocations:  []string{},
	}
}

// GetState retrieves the value for a given key from mock state
func (m *MockStub) GetState(key string) ([]byte, error) {
	m.Invocations = append(m.Invocations, fmt.Sprintf("GetState:%s", key))
	return m.State[key], nil
}

// PutState stores a key-value pair in mock state
func (m *MockStub) PutState(key string, value []byte) error {
	m.Invocations = append(m.Invocations, fmt.Sprintf("PutState:%s", key))
	m.State[key] = value
	return nil
}

// DelState removes a key from mock state
func (m *MockStub) DelState(key string) error {
	m.Invocations = append(m.Invocations, fmt.Sprintf("DeleteState:%s", key))
	delete(m.State, key)
	return nil
}

// GetStateByRange returns an iterator for keys within a range
func (m *MockStub) GetStateByRange(startKey, endKey string) (shim.StateQueryIteratorInterface, error) {
	return &MockIterator{}, nil
}

// GetStateByPartialCompositeKey returns an iterator for composite keys
func (m *MockStub) GetStateByPartialCompositeKey(objectType string, keys []string) (shim.StateQueryIteratorInterface, error) {
	return &MockIterator{}, nil
}

// GetQueryResult executes a rich query (not implemented in mock)
func (m *MockStub) GetQueryResult(query string) (shim.StateQueryIteratorInterface, error) {
	return &MockIterator{}, nil
}

// GetHistoryForKey returns history for a key (not implemented in mock)
func (m *MockStub) GetHistoryForKey(key string) (shim.HistoryQueryIteratorInterface, error) {
	return &MockHistoryIterator{}, nil
}

// CreateCompositeKey creates a composite key
func (m *MockStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return objectType + ":" + attributes[0], nil
}

// SplitCompositeKey splits a composite key
func (m *MockStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	return "", []string{}, nil
}

// GetTransient returns the transient map
func (m *MockStub) GetTransient() (map[string][]byte, error) {
	return m.TransientMap, nil
}

// GetTxID returns the transaction ID
func (m *MockStub) GetTxID() string {
	return m.TxID
}

// GetChannelID returns the channel ID
func (m *MockStub) GetChannelID() string {
	return m.ChannelID
}

// GetTxTimestamp returns a mock timestamp
func (m *MockStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	now := time.Now()
	return &timestamp.Timestamp{
		Seconds: now.Unix(),
		Nanos:   int32(now.Nanosecond()),
	}, nil
}

// GetCreator returns the transaction creator
func (m *MockStub) GetCreator() ([]byte, error) {
	return m.Creator, nil
}

// GetDecorations is a no-op
func (s *MockStub) GetDecorations() map[string][]byte {
	return make(map[string][]byte)
}

// GetBinding returns empty binding
func (m *MockStub) GetBinding() ([]byte, error) {
	return []byte{}, nil
}

// GetSignedProposal returns nil
func (m *MockStub) GetSignedProposal() (*peer.SignedProposal, error) {
	return nil, nil
}

// GetArgs returns empty args
func (m *MockStub) GetArgs() [][]byte {
	return [][]byte{}
}

// GetStringArgs returns empty args
func (m *MockStub) GetStringArgs() []string {
	return []string{}
}

// GetFunctionAndParameters returns mock function name
func (m *MockStub) GetFunctionAndParameters() (string, []string) {
	return "mockFunction", []string{}
}

// GetArgsSlice returns empty slice
func (m *MockStub) GetArgsSlice() ([]byte, error) {
	return []byte{}, nil
}

// SetEvent sets an event (no-op in mock)
func (m *MockStub) SetEvent(name string, payload []byte) error {
	return nil
}

// InvokeChaincode simulates chaincode invocation (not implemented)
func (m *MockStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) peer.Response {
	return shim.Success(nil)
}

// GetStateValidationParameter returns nil
func (m *MockStub) GetStateValidationParameter(key string) ([]byte, error) {
	return nil, nil
}

// SetStateValidationParameter is a no-op
func (m *MockStub) SetStateValidationParameter(key string, ep []byte) error {
	return nil
}

// GetPrivateData returns nil
func (m *MockStub) GetPrivateData(collection, key string) ([]byte, error) {
	return nil, nil
}

// GetPrivateDataHash is a no-op
func (s *MockStub) GetPrivateDataHash(collection string, key string) ([]byte, error) {
	return nil, nil
}

// PutPrivateData is a no-op
func (m *MockStub) PutPrivateData(collection, key string, value []byte) error {
	return nil
}

// DelPrivateData is a no-op
func (m *MockStub) DelPrivateData(collection, key string) error {
	return nil
}

// GetPrivateDataByRange returns empty iterator
func (m *MockStub) GetPrivateDataByRange(collection, startKey, endKey string) (shim.StateQueryIteratorInterface, error) {
	return &MockIterator{}, nil
}

// GetPrivateDataByPartialCompositeKey returns empty iterator
func (m *MockStub) GetPrivateDataByPartialCompositeKey(collection, objectType string, keys []string) (shim.StateQueryIteratorInterface, error) {
	return &MockIterator{}, nil
}

// GetPrivateDataQueryResult returns empty iterator
func (m *MockStub) GetPrivateDataQueryResult(collection, query string) (shim.StateQueryIteratorInterface, error) {
	return &MockIterator{}, nil
}

// GetPrivateDataValidationParameter returns nil
func (m *MockStub) GetPrivateDataValidationParameter(collection, key string) ([]byte, error) {
	return nil, nil
}

// SetPrivateDataValidationParameter is a no-op
func (m *MockStub) SetPrivateDataValidationParameter(collection, key string, ep []byte) error {
	return nil
}

// GetQueryResultWithPagination is a no-op
func (s *MockStub) GetQueryResultWithPagination(query string, pageSize int32, bookmark string) (shim.StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {
	return nil, nil, nil
}

// GetStateByPartialCompositeKeyWithPagination is a no-op
func (s *MockStub) GetStateByPartialCompositeKeyWithPagination(objectType string, keys []string, pageSize int32, bookmark string) (shim.StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {
	return nil, nil, nil
}

// GetStateByRangeWithPagination is a no-op
func (s *MockStub) GetStateByRangeWithPagination(startKey, endKey string, pageSize int32, bookmark string) (shim.StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {
	return nil, nil, nil
}

// PurgePrivateData is a no-op
func (s *MockStub) PurgePrivateData(collection string, key string) error {
	return nil
}

// //////////////////////////////////////////////////////
// MockIterator implements StateQueryIteratorInterface //
// //////////////////////////////////////////////////////
type MockIterator struct {
	current int
	items   []*queryresult.KV
}

func (m *MockIterator) HasNext() bool {
	return m.current < len(m.items)
}

func (m *MockIterator) Next() (*queryresult.KV, error) {
	if !m.HasNext() {
		return nil, fmt.Errorf("no more items")
	}
	item := m.items[m.current]
	m.current++
	return item, nil
}

func (m *MockIterator) Close() error {
	return nil
}

// ///////////////////////////////////////////////////////////////
// MockHistoryIterator implements HistoryQueryIteratorInterface //
// ///////////////////////////////////////////////////////////////
type MockHistoryIterator struct{}

func (m *MockHistoryIterator) HasNext() bool {
	return false
}

func (m *MockHistoryIterator) Next() (*queryresult.KeyModification, error) {
	return nil, fmt.Errorf("no history")
}

func (m *MockHistoryIterator) Close() error {
	return nil
}

// ////////////////////////////////////////////////////////////
// MockStubWrapper wraps MockStub for cc-tools compatibility //
// ////////////////////////////////////////////////////////////
type MockStubWrapper struct {
	*sw.StubWrapper
	mockStub *MockStub
}

// NewMockStubWrapper creates a wrapped mock stub
func NewMockStubWrapper() (*MockStubWrapper, *MockStub) {
	mockStub := NewMockStub()
	wrapper := &sw.StubWrapper{
		Stub: mockStub,
	}
	return &MockStubWrapper{
		StubWrapper: wrapper,
		mockStub:    mockStub,
	}, mockStub
}

// GetMockStub returns the underlying mock stub for assertions
func (m *MockStubWrapper) GetMockStub() *MockStub {
	return m.mockStub
}
